package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ resource.Resource = &NamespaceResource{}
var _ resource.ResourceWithImportState = &NamespaceResource{}

type NamespaceResource struct {
	client *client.Client
}

type NamespaceResourceModel struct {
	Path            types.String `tfsdk:"path"`
	State           types.String `tfsdk:"state"`
	EncryptionKeyId types.String `tfsdk:"encryption_key_id"`
	Labels          types.Map    `tfsdk:"labels"`
	Annotations     types.Map    `tfsdk:"annotations"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func NewNamespaceResource() resource.Resource {
	return &NamespaceResource{}
}

func (r *NamespaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (r *NamespaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy namespace.",
		Attributes: map[string]schema.Attribute{
			"path": schema.StringAttribute{
				Description: "The namespace path (e.g. 'root.production').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Description: "The namespace state.",
				Computed:    true,
			},
			"encryption_key_id": schema.StringAttribute{
				Description: "The encryption key ID associated with this namespace.",
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the namespace.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations for the namespace.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *NamespaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

func (r *NamespaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NamespaceResourceModel
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

	ns, err := r.client.CreateNamespace(ctx, client.CreateNamespaceRequest{
		Path:        plan.Path.ValueString(),
		Labels:      labels,
		Annotations: annotations,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create namespace", err.Error())
		return
	}

	setNamespaceState(&plan, ns)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NamespaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NamespaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ns, err := r.client.GetNamespace(ctx, state.Path.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read namespace", err.Error())
		return
	}

	setNamespaceState(&state, ns)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NamespaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NamespaceResourceModel
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

	ns, err := r.client.UpdateNamespace(ctx, plan.Path.ValueString(), client.UpdateNamespaceRequest{
		Labels:      labels,
		Annotations: annotations,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update namespace", err.Error())
		return
	}

	setNamespaceState(&plan, ns)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NamespaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NamespaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteNamespace(ctx, state.Path.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete namespace", err.Error())
	}
}

func (r *NamespaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("path"), req.ID)...)
}

func setNamespaceState(model *NamespaceResourceModel, ns *client.Namespace) {
	model.Path = types.StringValue(ns.Path)
	model.State = types.StringValue(ns.State)
	if ns.EncryptionKeyId != nil {
		model.EncryptionKeyId = types.StringValue(*ns.EncryptionKeyId)
	} else {
		model.EncryptionKeyId = types.StringNull()
	}
	model.Labels = labelsToMap(ns.Labels)
	model.Annotations = annotationsToMap(ns.Annotations)
	model.CreatedAt = types.StringValue(ns.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	model.UpdatedAt = types.StringValue(ns.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}
