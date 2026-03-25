package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ datasource.DataSource = &ConnectorDataSource{}

type ConnectorDataSource struct {
	client *client.Client
}

type ConnectorDataSourceModel struct {
	Id          types.String `tfsdk:"id"`
	Namespace   types.String `tfsdk:"namespace"`
	Version     types.Int64  `tfsdk:"version"`
	State       types.String `tfsdk:"state"`
	DisplayName types.String `tfsdk:"display_name"`
	Description types.String `tfsdk:"description"`
	Logo        types.String `tfsdk:"logo"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

func NewConnectorDataSource() datasource.DataSource {
	return &ConnectorDataSource{}
}

func (d *ConnectorDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connector"
}

func (d *ConnectorDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an AuthProxy connector.",
		Attributes: map[string]schema.Attribute{
			"id":           schema.StringAttribute{Required: true},
			"namespace":    schema.StringAttribute{Computed: true},
			"version":      schema.Int64Attribute{Computed: true},
			"state":        schema.StringAttribute{Computed: true},
			"display_name": schema.StringAttribute{Computed: true},
			"description":  schema.StringAttribute{Computed: true},
			"logo":         schema.StringAttribute{Computed: true},
			"labels":       schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"annotations":  schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"created_at":   schema.StringAttribute{Computed: true},
			"updated_at":   schema.StringAttribute{Computed: true},
		},
	}
}

func (d *ConnectorDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*client.Client)
}

func (d *ConnectorDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ConnectorDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := d.client.GetConnector(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read connector", err.Error())
		return
	}

	config.Namespace = types.StringValue(conn.Namespace)
	config.Version = types.Int64Value(int64(conn.Version))
	config.State = types.StringValue(conn.State)
	config.DisplayName = types.StringValue(conn.DisplayName)
	config.Description = types.StringValue(conn.Description)
	config.Logo = types.StringValue(conn.Logo)
	config.Labels = labelsToMap(conn.Labels)
	config.Annotations = annotationsToMap(conn.Annotations)
	config.CreatedAt = types.StringValue(conn.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	config.UpdatedAt = types.StringValue(conn.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
