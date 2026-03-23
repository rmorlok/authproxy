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

var _ resource.Resource = &EncryptionKeyResource{}
var _ resource.ResourceWithImportState = &EncryptionKeyResource{}

type EncryptionKeyResource struct {
	client *client.Client
}

type EncryptionKeyResourceModel struct {
	Id        types.String `tfsdk:"id"`
	Namespace types.String `tfsdk:"namespace"`
	State     types.String `tfsdk:"state"`
	Labels    types.Map    `tfsdk:"labels"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func NewEncryptionKeyResource() resource.Resource {
	return &EncryptionKeyResource{}
}

func (r *EncryptionKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encryption_key"
}

func (r *EncryptionKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy encryption key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The encryption key ID.",
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
				Description: "The encryption key state (active, disabled).",
				Optional:    true,
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the encryption key.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *EncryptionKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

func (r *EncryptionKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EncryptionKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ek, err := r.client.CreateEncryptionKey(ctx, client.CreateEncryptionKeyRequest{
		Namespace: plan.Namespace.ValueString(),
		Labels:    labels,
		KeyData:   map[string]interface{}{"random": true},
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create encryption key", err.Error())
		return
	}

	setEncryptionKeyState(&plan, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EncryptionKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EncryptionKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ek, err := r.client.GetEncryptionKey(ctx, state.Id.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read encryption key", err.Error())
		return
	}

	setEncryptionKeyState(&state, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *EncryptionKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan EncryptionKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	labels := extractLabels(ctx, plan.Labels, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := client.UpdateEncryptionKeyRequest{}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		s := plan.State.ValueString()
		updateReq.State = &s
	}
	if labels != nil {
		updateReq.Labels = &labels
	}

	ek, err := r.client.UpdateEncryptionKey(ctx, plan.Id.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update encryption key", err.Error())
		return
	}

	setEncryptionKeyState(&plan, ek)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EncryptionKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EncryptionKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteEncryptionKey(ctx, state.Id.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete encryption key", err.Error())
	}
}

func (r *EncryptionKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("id"), req.ID)...)
}

func setEncryptionKeyState(model *EncryptionKeyResourceModel, ek *client.EncryptionKey) {
	model.Id = types.StringValue(ek.Id)
	model.Namespace = types.StringValue(ek.Namespace)
	model.State = types.StringValue(ek.State)
	model.Labels = labelsToMap(ek.Labels)
	model.CreatedAt = types.StringValue(ek.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	model.UpdatedAt = types.StringValue(ek.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}
