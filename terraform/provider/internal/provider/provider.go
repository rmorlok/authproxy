package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
	"github.com/rmorlok/authproxy/terraform/provider/internal/datasources"
	"github.com/rmorlok/authproxy/terraform/provider/internal/resources"
)

var _ provider.Provider = &AuthProxyProvider{}

type AuthProxyProvider struct {
	version string
}

type AuthProxyProviderModel struct {
	Endpoint       types.String `tfsdk:"endpoint"`
	BearerToken    types.String `tfsdk:"bearer_token"`
	PrivateKeyPath types.String `tfsdk:"private_key_path"`
	Username       types.String `tfsdk:"username"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &AuthProxyProvider{version: version}
	}
}

func (p *AuthProxyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "authproxy"
	resp.Version = p.version
}

func (p *AuthProxyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing AuthProxy resources (namespaces, encryption keys, actors, connectors).",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "The AuthProxy admin API endpoint URL (e.g. http://localhost:8082). Can also be set with AUTHPROXY_ENDPOINT.",
				Optional:    true,
			},
			"bearer_token": schema.StringAttribute{
				Description: "Pre-signed JWT bearer token for authentication. Can also be set with AUTHPROXY_BEARER_TOKEN.",
				Optional:    true,
				Sensitive:   true,
			},
			"private_key_path": schema.StringAttribute{
				Description: "Path to a private key file for JWT signing. Can also be set with AUTHPROXY_PRIVATE_KEY_PATH.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username (actor external ID) for JWT claims. Required with private_key_path. Can also be set with AUTHPROXY_USERNAME.",
				Optional:    true,
			},
		},
	}
}

func (p *AuthProxyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config AuthProxyProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := stringValueOrEnv(config.Endpoint, "AUTHPROXY_ENDPOINT")
	bearerToken := stringValueOrEnv(config.BearerToken, "AUTHPROXY_BEARER_TOKEN")
	privateKeyPath := stringValueOrEnv(config.PrivateKeyPath, "AUTHPROXY_PRIVATE_KEY_PATH")
	username := stringValueOrEnv(config.Username, "AUTHPROXY_USERNAME")

	if endpoint == "" {
		resp.Diagnostics.AddError("Missing endpoint", "The provider endpoint must be set via the 'endpoint' attribute or AUTHPROXY_ENDPOINT environment variable.")
		return
	}

	if bearerToken == "" && privateKeyPath == "" {
		resp.Diagnostics.AddError("Missing authentication", "Either 'bearer_token' or 'private_key_path' must be set (or their AUTHPROXY_* environment variables).")
		return
	}

	if privateKeyPath != "" && username == "" {
		resp.Diagnostics.AddError("Missing username", "The 'username' attribute is required when using 'private_key_path'.")
		return
	}

	c, err := client.New(client.Config{
		Endpoint:       endpoint,
		BearerToken:    bearerToken,
		PrivateKeyPath: privateKeyPath,
		Username:       username,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create client", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *AuthProxyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewNamespaceResource,
		resources.NewEncryptionKeyResource,
		resources.NewActorResource,
		resources.NewConnectorResource,
	}
}

func (p *AuthProxyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewNamespaceDataSource,
		datasources.NewEncryptionKeyDataSource,
		datasources.NewActorDataSource,
		datasources.NewConnectorDataSource,
	}
}

func stringValueOrEnv(val types.String, envKey string) string {
	if !val.IsNull() && !val.IsUnknown() {
		return val.ValueString()
	}
	return os.Getenv(envKey)
}
