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

var _ resource.Resource = &ActorResource{}
var _ resource.ResourceWithImportState = &ActorResource{}

type ActorResource struct {
	client *client.Client
}

type ActorResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Namespace   types.String `tfsdk:"namespace"`
	ExternalId  types.String `tfsdk:"external_id"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewActorResource() resource.Resource {
	return &ActorResource{}
}

func (r *ActorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_actor"
}

func (r *ActorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an AuthProxy actor.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The actor ID.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The namespace this actor belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"external_id": schema.StringAttribute{
				Description: "The external identifier for this actor.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"labels": schema.MapAttribute{
				Description: "Labels for the actor.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"annotations": schema.MapAttribute{
				Description: "Annotations for the actor.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *ActorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*client.Client)
}

func (r *ActorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ActorResourceModel
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

	a, err := r.client.CreateActor(ctx, client.CreateActorRequest{
		ExternalId:  plan.ExternalId.ValueString(),
		Namespace:   plan.Namespace.ValueString(),
		Labels:      labels,
		Annotations: annotations,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create actor", err.Error())
		return
	}

	setActorState(&plan, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ActorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ActorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := r.client.GetActor(ctx, state.Id.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read actor", err.Error())
		return
	}

	setActorState(&state, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ActorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ActorResourceModel
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

	a, err := r.client.UpdateActor(ctx, plan.Id.ValueString(), client.UpdateActorRequest{
		Labels:      labels,
		Annotations: annotations,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update actor", err.Error())
		return
	}

	setActorState(&plan, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ActorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ActorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteActor(ctx, state.Id.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete actor", err.Error())
	}
}

func (r *ActorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, pathAttr("id"), req.ID)...)
}

func setActorState(model *ActorResourceModel, a *client.Actor) {
	model.Id = types.StringValue(a.Id)
	model.Namespace = types.StringValue(a.Namespace)
	model.ExternalId = types.StringValue(a.ExternalId)
	model.Labels = labelsToMap(a.Labels)
	model.Annotations = annotationsToMap(a.Annotations)
	model.CreatedAt = types.StringValue(a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	model.UpdatedAt = types.StringValue(a.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
}
