package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ datasource.DataSource = &ActorDataSource{}

type ActorDataSource struct {
	client *client.Client
}

type ActorDataSourceModel struct {
	Id         types.String `tfsdk:"id"`
	Namespace  types.String `tfsdk:"namespace"`
	ExternalId types.String `tfsdk:"external_id"`
	Labels     types.Map    `tfsdk:"labels"`
	CreatedAt  types.String `tfsdk:"created_at"`
	UpdatedAt  types.String `tfsdk:"updated_at"`
}

func NewActorDataSource() datasource.DataSource {
	return &ActorDataSource{}
}

func (d *ActorDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_actor"
}

func (d *ActorDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an AuthProxy actor.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Required: true},
			"namespace":   schema.StringAttribute{Computed: true},
			"external_id": schema.StringAttribute{Computed: true},
			"labels":      schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"created_at":  schema.StringAttribute{Computed: true},
			"updated_at":  schema.StringAttribute{Computed: true},
		},
	}
}

func (d *ActorDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*client.Client)
}

func (d *ActorDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ActorDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	a, err := d.client.GetActor(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read actor", err.Error())
		return
	}

	config.Namespace = types.StringValue(a.Namespace)
	config.ExternalId = types.StringValue(a.ExternalId)
	config.Labels = labelsToMap(a.Labels)
	config.CreatedAt = types.StringValue(a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	config.UpdatedAt = types.StringValue(a.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
