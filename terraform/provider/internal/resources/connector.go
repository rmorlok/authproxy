package resources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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
		Description: "Manages an AuthProxy connector. Automatically handles version lifecycle: when the definition changes, a new version is created and optionally promoted to primary.",
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
				CustomType:  jsontypes.NormalizedType{},
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the connector.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"publish": schema.BoolAttribute{
				Description: "Whether to promote new versions to primary state. When true (default), new versions are automatically set as primary. When false, versions remain in draft state.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"version": schema.Int64Attribute{
				Description: "The current version number.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
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

	defJSON := json.RawMessage(plan.Definition.ValueString())

	cv, err := r.client.CreateConnector(ctx, client.CreateConnectorRequest{
		Namespace:  plan.Namespace.ValueString(),
		Definition: defJSON,
		Labels:     labels,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create connector", err.Error())
		return
	}

	// If publish is true, promote to primary
	if plan.Publish.ValueBool() {
		err = r.client.ForceConnectorVersionState(ctx, cv.Id, cv.Version, "primary")
		if err != nil {
			resp.Diagnostics.AddError("Failed to promote connector to primary", err.Error())
			return
		}
		// Re-read to get updated state
		cv, err = r.client.GetConnectorVersion(ctx, cv.Id, cv.Version)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read connector after promotion", err.Error())
			return
		}
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

	// Read the connector to get the latest version
	conn, err := r.client.GetConnector(ctx, id)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read connector", err.Error())
		return
	}

	// Get the full version details (with definition)
	cv, err := r.client.GetConnectorVersion(ctx, id, conn.Version)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read connector version", err.Error())
		return
	}

	setConnectorState(&state, cv)
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

	definitionChanged := !plan.Definition.Equal(state.Definition)
	publishChanged := !plan.Publish.Equal(state.Publish)

	var cv *client.ConnectorVersion
	var err error

	if definitionChanged {
		// Definition changed: create a new version
		defJSON := json.RawMessage(plan.Definition.ValueString())
		labelsPtr := &labels

		cv, err = r.client.CreateConnectorVersion(ctx, id, client.CreateConnectorVersionRequest{
			Definition: &defJSON,
			Labels:     labelsPtr,
		})
		if err != nil {
			resp.Diagnostics.AddError("Failed to create new connector version", err.Error())
			return
		}

		if plan.Publish.ValueBool() {
			err = r.client.ForceConnectorVersionState(ctx, id, cv.Version, "primary")
			if err != nil {
				resp.Diagnostics.AddError("Failed to promote new version to primary", err.Error())
				return
			}
			cv, err = r.client.GetConnectorVersion(ctx, id, cv.Version)
			if err != nil {
				resp.Diagnostics.AddError("Failed to read connector version after promotion", err.Error())
				return
			}
		}
	} else if publishChanged && plan.Publish.ValueBool() {
		// Publish changed from false to true: promote current draft version
		currentVersion := uint64(state.Version.ValueInt64())
		err = r.client.ForceConnectorVersionState(ctx, id, currentVersion, "primary")
		if err != nil {
			resp.Diagnostics.AddError("Failed to promote version to primary", err.Error())
			return
		}
		cv, err = r.client.GetConnectorVersion(ctx, id, currentVersion)
		if err != nil {
			resp.Diagnostics.AddError("Failed to read connector version after promotion", err.Error())
			return
		}
	} else {
		// Only labels changed: update current version
		currentVersion := uint64(state.Version.ValueInt64())
		updateReq := client.UpdateConnectorRequest{Labels: &labels}
		cv, err = r.client.UpdateConnectorVersion(ctx, id, currentVersion, updateReq)
		if err != nil {
			resp.Diagnostics.AddError("Failed to update connector labels", err.Error())
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
		if v.State != "archived" {
			err = r.client.ForceConnectorVersionState(ctx, id, v.Version, "archived")
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

// connectorMetadataFields are fields the API adds to the definition response
// that are not part of the user-provided definition. These must be stripped
// before storing the definition in state to avoid "inconsistent result" errors.
var connectorMetadataFields = []string{
	"id", "version", "namespace", "state", "logo",
	"labels", "created_at", "updated_at",
}

func setConnectorState(model *ConnectorResourceModel, cv *client.ConnectorVersion) {
	model.Id = types.StringValue(cv.Id)
	model.Namespace = types.StringValue(cv.Namespace)
	model.Version = types.Int64Value(int64(cv.Version))
	model.State = types.StringValue(cv.State)
	model.Labels = labelsToMap(cv.Labels)
	model.CreatedAt = types.StringValue(cv.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	model.UpdatedAt = types.StringValue(cv.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	if cv.Definition != nil {
		// Strip metadata fields that the API adds but aren't part of the
		// user-provided definition.
		var defMap map[string]json.RawMessage
		if err := json.Unmarshal(cv.Definition, &defMap); err == nil {
			for _, field := range connectorMetadataFields {
				delete(defMap, field)
			}
			if cleaned, err := json.Marshal(defMap); err == nil {
				model.Definition = jsontypes.NewNormalizedValue(string(cleaned))
			} else {
				model.Definition = jsontypes.NewNormalizedValue(string(cv.Definition))
			}
		} else {
			model.Definition = jsontypes.NewNormalizedValue(string(cv.Definition))
		}
	}

	// Extract display_name from definition
	var def struct {
		DisplayName string `json:"display_name"`
	}
	if cv.Definition != nil {
		if err := json.Unmarshal(cv.Definition, &def); err == nil {
			model.DisplayName = types.StringValue(def.DisplayName)
		}
	}
}
