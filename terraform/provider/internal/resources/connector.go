package resources

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ resource.Resource = &ConnectorResource{}
var _ resource.ResourceWithImportState = &ConnectorResource{}

type ConnectorResource struct {
	client *client.Client
}

type ConnectorResourceModel struct {
	Id          types.String         `tfsdk:"id"`
	Namespace   types.String         `tfsdk:"namespace"`
	Definition  jsontypes.Normalized `tfsdk:"definition"`
	Labels      types.Map            `tfsdk:"labels"`
	Annotations types.Map            `tfsdk:"annotations"`
	Publish     types.Bool           `tfsdk:"publish"`
	Version     types.Int64          `tfsdk:"version"`
	State       types.String         `tfsdk:"state"`
	DisplayName types.String         `tfsdk:"display_name"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func NewConnectorResource() resource.Resource {
	return &ConnectorResource{}
}

func (r *ConnectorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connector"
}

func (r *ConnectorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy connector and its generation lifecycle. Published definition changes create a new generation; draft definition changes update that draft in place.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The stable connector ID (persists across version changes).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The namespace this connector belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"definition": schema.StringAttribute{
				Description: "The connector definition as JSON (auth config, probes, rate limiting, etc.).",
				Required:    true,
				Sensitive:   true,
				CustomType:  jsontypes.NormalizedType{},
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the connector.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations for the connector.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"publish": schema.BoolAttribute{
				Description: "Whether to publish newly created or currently managed draft generations. Changing this from true to false does not demote an already published generation.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"version": schema.Int64Attribute{
				Description: "The current API metadata.generation, exposed under the provider's established version attribute.",
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "The current version state (draft, primary, active, archived).",
				Computed:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "Display name extracted from the definition.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *ConnectorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

func (r *ConnectorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ConnectorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	annotations := extractAnnotations(ctx, plan.Annotations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	defJSON := json.RawMessage(plan.Definition.ValueString())

	cv, err := r.client.CreateConnector(ctx, client.CreateConnectorRequest{
		TypeMeta: client.NewTypeMeta(client.ConnectorKind),
		Metadata: client.ObjectMetadata{
			Namespace:   plan.Namespace.ValueString(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: client.ConnectorSpec{
			Release:    client.ConnectorReleaseSpec{DesiredState: desiredConnectorReleaseState(plan.Publish.ValueBool())},
			Definition: defJSON,
		},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create connector", err.Error())
		return
	}

	setConnectorState(&plan, cv)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ConnectorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.Id.ValueString()

	var connector *client.Connector
	var err error
	// A draft is not the logical connector's primary generation. Preserve the
	// exact generation managed by Terraform; published resources deliberately
	// follow the API's primary generation so out-of-band promotion is detected.
	if !state.Publish.IsNull() && !state.Publish.IsUnknown() && !state.Publish.ValueBool() &&
		!state.Version.IsNull() && !state.Version.IsUnknown() && state.Version.ValueInt64() > 0 {
		connector, err = r.client.GetConnectorVersion(ctx, id, uint64(state.Version.ValueInt64()))
	} else {
		connector, err = r.client.GetConnector(ctx, id)
	}
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read connector", err.Error())
		return
	}

	setConnectorState(&state, connector)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ConnectorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ConnectorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.Id.ValueString()
	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	annotations := extractAnnotations(ctx, plan.Annotations, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	definitionChanged := !plan.Definition.Equal(state.Definition)
	publishChanged := !plan.Publish.Equal(state.Publish)

	var cv *client.Connector
	var err error

	if definitionChanged {
		defJSON := json.RawMessage(plan.Definition.ValueString())
		if state.State.ValueString() == "draft" {
			desiredState := desiredConnectorReleaseState(plan.Publish.ValueBool())
			cv, err = r.client.UpdateConnectorVersion(ctx, id, uint64(state.Version.ValueInt64()), client.UpdateConnectorRequest{
				TypeMeta: client.NewTypeMeta(client.ConnectorKind),
				Metadata: connectorMetadataPatch(labels, annotations),
				Spec: &client.ConnectorSpecPatch{
					Release:    &client.ConnectorReleaseSpecPatch{DesiredState: &desiredState},
					Definition: &defJSON,
				},
			})
		} else {
			cv, err = r.client.CreateConnectorVersion(ctx, id, client.CreateConnectorVersionRequest{
				TypeMeta: client.NewTypeMeta(client.ConnectorKind),
				Metadata: client.ObjectMetadata{
					Namespace:   plan.Namespace.ValueString(),
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: client.ConnectorSpec{
					Release:    client.ConnectorReleaseSpec{DesiredState: desiredConnectorReleaseState(plan.Publish.ValueBool())},
					Definition: defJSON,
				},
			})
		}
		if err != nil {
			resp.Diagnostics.AddError("Failed to update connector definition", err.Error())
			return
		}
	} else if publishChanged && plan.Publish.ValueBool() {
		// Publish changed from false to true: promote the exact draft generation.
		currentVersion := uint64(state.Version.ValueInt64())
		desiredState := "primary"
		cv, err = r.client.UpdateConnectorVersion(ctx, id, currentVersion, client.UpdateConnectorRequest{
			TypeMeta: client.NewTypeMeta(client.ConnectorKind),
			Metadata: connectorMetadataPatch(labels, annotations),
			Spec: &client.ConnectorSpecPatch{Release: &client.ConnectorReleaseSpecPatch{
				DesiredState: &desiredState,
			}},
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to promote version to primary", err.Error())
			return
		}
	} else {
		updateReq := client.UpdateConnectorRequest{
			TypeMeta: client.NewTypeMeta(client.ConnectorKind),
			Metadata: connectorMetadataPatch(labels, annotations),
			Spec:     &client.ConnectorSpecPatch{},
		}
		if state.State.ValueString() == "draft" {
			cv, err = r.client.UpdateConnectorVersion(ctx, id, uint64(state.Version.ValueInt64()), updateReq)
		} else {
			cv, err = r.client.UpdateConnector(ctx, id, updateReq)
		}
		if err != nil {
			resp.Diagnostics.AddError("Failed to update connector metadata", err.Error())
			return
		}
	}

	setConnectorState(&plan, cv)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ConnectorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ConnectorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.Id.ValueString()

	// List all versions and archive non-archived ones
	versions, err := r.client.ListConnectorVersions(ctx, id)
	if err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to list connector versions", err.Error())
		return
	}

	for _, v := range versions.Items {
		if v.Status != nil && v.Status.Release.State != "archived" {
			err = r.client.ForceConnectorVersionState(ctx, id, v.Metadata.Generation, "archived")
			if err != nil {
				resp.Diagnostics.AddError("Failed to archive connector version", err.Error())
				return
			}
		}
	}
}

func (r *ConnectorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("id"), req.ID)...)
	// Set publish to true as default on import
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("publish"), true)...)
}

func setConnectorState(model *ConnectorResourceModel, connector *client.Connector) {
	model.Id = types.StringValue(connector.Metadata.ID)
	model.Namespace = types.StringValue(connector.Metadata.Namespace)
	model.Version = types.Int64Value(int64(connector.Metadata.Generation))
	model.Labels = labelsToMap(connector.Metadata.Labels)
	model.Annotations = annotationsToMap(connector.Metadata.Annotations)
	model.CreatedAt = timestampToString(connector.Metadata.CreatedAt)
	model.UpdatedAt = timestampToString(connector.Metadata.UpdatedAt)
	if connector.Status != nil {
		model.State = types.StringValue(connector.Status.Release.State)
	} else {
		model.State = types.StringNull()
	}

	if len(connector.Spec.Definition) > 0 {
		definition := connector.Spec.Definition
		if connector.DataRedacted && !model.Definition.IsNull() && !model.Definition.IsUnknown() {
			if merged, err := preserveRedactedJSON(definition, []byte(model.Definition.ValueString())); err == nil {
				definition = merged
			}
		}
		model.Definition = jsontypes.NewNormalizedValue(string(definition))
		if summary, err := client.DecodeConnectorDefinitionSummary(connector.Spec.Definition); err == nil {
			model.DisplayName = types.StringValue(summary.DisplayName)
		}
	}
}

// preserveRedactedJSON keeps prior Terraform state at fields the API explicitly
// masked in a response. This prevents write-only connector secrets from being
// replaced with asterisks after apply while leaving all observable fields
// available for drift detection.
func preserveRedactedJSON(observed, prior []byte) (json.RawMessage, error) {
	var observedValue any
	if err := json.Unmarshal(observed, &observedValue); err != nil {
		return nil, err
	}
	var priorValue any
	if err := json.Unmarshal(prior, &priorValue); err != nil {
		return nil, err
	}
	merged := preserveRedactedValue(observedValue, priorValue)
	return json.Marshal(merged)
}

func preserveRedactedValue(observed, prior any) any {
	switch value := observed.(type) {
	case string:
		if value != "" && strings.Trim(value, "*") == "" {
			if priorString, ok := prior.(string); ok &&
				utf8.RuneCountInString(priorString) == utf8.RuneCountInString(value) {
				return priorString
			}
		}
	case map[string]any:
		priorMap, _ := prior.(map[string]any)
		for key, item := range value {
			value[key] = preserveRedactedValue(item, priorMap[key])
		}
	case []any:
		priorSlice, _ := prior.([]any)
		for index, item := range value {
			var priorItem any
			if index < len(priorSlice) {
				priorItem = priorSlice[index]
			}
			value[index] = preserveRedactedValue(item, priorItem)
		}
	}
	return observed
}

func desiredConnectorReleaseState(publish bool) string {
	if publish {
		return "primary"
	}
	return "draft"
}

func connectorMetadataPatch(labels, annotations map[string]string) *client.ObjectMetadataPatch {
	patch := &client.ObjectMetadataPatch{}
	if labels != nil {
		patch.Labels = &labels
	}
	if annotations != nil {
		patch.Annotations = &annotations
	}
	return patch
}
