package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ datasource.DataSource = &NamespaceDataSource{}

type NamespaceDataSource struct {
	client *client.Client
}

type NamespaceDataSourceModel struct {
	Path            types.String `tfsdk:"path"`
	State           types.String `tfsdk:"state"`
	EncryptionKeyId types.String `tfsdk:"encryption_key_id"`
	Labels          types.Map    `tfsdk:"labels"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}

func NewNamespaceDataSource() datasource.DataSource {
	return &NamespaceDataSource{}
}

func (d *NamespaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_namespace"
}

func (d *NamespaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an AuthProxy namespace.",
		Attributes: map[string]schema.Attribute{
			"path":              schema.StringAttribute{Required: true},
			"state":             schema.StringAttribute{Computed: true},
			"encryption_key_id": schema.StringAttribute{Computed: true},
			"labels":            schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"created_at":        schema.StringAttribute{Computed: true},
			"updated_at":        schema.StringAttribute{Computed: true},
		},
	}
}

func (d *NamespaceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*client.Client)
}

func (d *NamespaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config NamespaceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ns, err := d.client.GetNamespace(ctx, config.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read namespace", err.Error())
		return
	}

	config.State = types.StringValue(ns.State)
	if ns.EncryptionKeyId != nil {
		config.EncryptionKeyId = types.StringValue(*ns.EncryptionKeyId)
	} else {
		config.EncryptionKeyId = types.StringNull()
	}
	config.Labels = labelsToMap(ns.Labels)
	config.CreatedAt = types.StringValue(ns.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	config.UpdatedAt = types.StringValue(ns.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
