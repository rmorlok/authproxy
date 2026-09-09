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
	Generation  types.Int64  `tfsdk:"generation"`
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
			"generation":   schema.Int64Attribute{Computed: true, Description: "The selected connector metadata.generation."},
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

	config.Namespace = types.StringValue(conn.Metadata.Namespace)
	config.Generation = types.Int64Value(int64(conn.Metadata.Generation))
	if conn.Status != nil {
		config.State = types.StringValue(conn.Status.Release.State)
	} else {
		config.State = types.StringNull()
	}
	if summary, err := client.DecodeConnectorDefinitionSummary(conn.Spec.Definition); err == nil {
		config.DisplayName = types.StringValue(summary.DisplayName)
		config.Description = types.StringValue(summary.Description)
		config.Logo = types.StringValue(summary.Logo)
	} else {
		resp.Diagnostics.AddError("Failed to decode connector definition", err.Error())
		return
	}
	config.Labels = labelsToMap(conn.Metadata.Labels)
	config.Annotations = annotationsToMap(conn.Metadata.Annotations)
	config.CreatedAt = timestampToString(conn.Metadata.CreatedAt)
	config.UpdatedAt = timestampToString(conn.Metadata.UpdatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
