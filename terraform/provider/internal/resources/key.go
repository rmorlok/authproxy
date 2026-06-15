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

var _ resource.Resource = &KeyResource{}
var _ resource.ResourceWithImportState = &KeyResource{}

type KeyResource struct {
	client *client.Client
}

type KeyResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Namespace   types.String `tfsdk:"namespace"`
	State       types.String `tfsdk:"state"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewKeyResource() resource.Resource {
	return &KeyResource{}
}

func (r *KeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_key"
}

func (r *KeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The key ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The namespace this key belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Description: "The key state (active, disabled).",
				Optional:    true,
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the key.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations for the key.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *KeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

func (r *KeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan KeyResourceModel
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

	ek, err := r.client.CreateKey(ctx, client.CreateKeyRequest{
		Namespace:   plan.Namespace.ValueString(),
		Labels:      labels,
		Annotations: annotations,
		KeyData:     map[string]interface{}{"random": true},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create key", err.Error())
		return
	}

	setKeyState(&plan, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *KeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state KeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ek, err := r.client.GetKey(ctx, state.Id.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read key", err.Error())
		return
	}

	setKeyState(&state, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *KeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan KeyResourceModel
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

	updateReq := client.UpdateKeyRequest{}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		s := plan.State.ValueString()
		updateReq.State = &s
	}
	if labels != nil {
		updateReq.Labels = &labels
	}
	if annotations != nil {
		updateReq.Annotations = &annotations
	}

	ek, err := r.client.UpdateKey(ctx, plan.Id.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update key", err.Error())
		return
	}

	setKeyState(&plan, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *KeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state KeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteKey(ctx, state.Id.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete key", err.Error())
	}
}

func (r *KeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("id"), req.ID)...)
}

func setKeyState(model *KeyResourceModel, ek *client.Key) {
	model.Id = types.StringValue(ek.Id)
	model.Namespace = types.StringValue(ek.Namespace)
	model.State = types.StringValue(ek.State)
	model.Labels = labelsToMap(ek.Labels)
	model.Annotations = annotationsToMap(ek.Annotations)
	model.CreatedAt = types.StringValue(ek.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	model.UpdatedAt = types.StringValue(ek.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}
