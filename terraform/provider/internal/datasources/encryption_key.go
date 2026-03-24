package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

var _ datasource.DataSource = &EncryptionKeyDataSource{}

type EncryptionKeyDataSource struct {
	client *client.Client
}

type EncryptionKeyDataSourceModel struct {
	Id        types.String `tfsdk:"id"`
	Namespace types.String `tfsdk:"namespace"`
	State     types.String `tfsdk:"state"`
	Labels    types.Map    `tfsdk:"labels"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func NewEncryptionKeyDataSource() datasource.DataSource {
	return &EncryptionKeyDataSource{}
}

func (d *EncryptionKeyDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_encryption_key"
}

func (d *EncryptionKeyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an AuthProxy encryption key.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Required: true},
			"namespace":  schema.StringAttribute{Computed: true},
			"state":      schema.StringAttribute{Computed: true},
			"labels":     schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *EncryptionKeyDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(*client.Client)
}

func (d *EncryptionKeyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EncryptionKeyDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ek, err := d.client.GetEncryptionKey(ctx, config.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read encryption key", err.Error())
		return
	}

	config.Namespace = types.StringValue(ek.Namespace)
	config.State = types.StringValue(ek.State)
	config.Labels = labelsToMap(ek.Labels)
	config.CreatedAt = types.StringValue(ek.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	config.UpdatedAt = types.StringValue(ek.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
