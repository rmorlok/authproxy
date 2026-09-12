# AuthProxy Terraform Provider

The AuthProxy Terraform provider enables managing AuthProxy resources through Infrastructure as Code. It communicates with the AuthProxy admin API over HTTP.

## Configuration

```hcl
provider "authproxy" {
  endpoint     = "http://localhost:8082"   # Admin API URL (or AUTHPROXY_ENDPOINT)
  bearer_token = "eyJ..."                  # Pre-signed JWT (or AUTHPROXY_BEARER_TOKEN)
}
```

### Authentication

The provider supports two authentication methods:

1. **Bearer token** (recommended for CI/CD): Set `bearer_token` or the `AUTHPROXY_BEARER_TOKEN` environment variable.

2. **Private key signing**: Set `private_key_path` and `username` to have the provider sign JWTs on-the-fly.

```hcl
provider "authproxy" {
  endpoint         = "http://localhost:8082"
  private_key_path = "/path/to/private/key"   # Or AUTHPROXY_PRIVATE_KEY_PATH
  username         = "admin"                   # Or AUTHPROXY_USERNAME
}
```

## Resources

- `authproxy_namespace` - Manages namespaces for multi-tenancy
- `authproxy_key` - Manages keys
- `authproxy_actor` - Manages actors (users/entities that own connections)
- `authproxy_connector` - Manages connectors with automatic generation lifecycle
- `authproxy_rate_limit` - Manages namespace-scoped rate-limit policies

## Data Sources

- `data.authproxy_namespace` - Reads an existing namespace
- `data.authproxy_key` - Reads an existing key
- `data.authproxy_actor` - Reads an existing actor
- `data.authproxy_connector` - Reads an existing connector

## API Resource Mapping

The provider keeps its idiomatic HCL interface while communicating with the
admin API using `authproxy.net/v1alpha1` resources. Users do not write the API
envelope in HCL; the provider maps fields as follows:

| Terraform value | API field |
|---|---|
| Resource or namespace ID/path | `metadata.id` |
| Namespace ownership | `metadata.namespace` |
| Labels and annotations | `metadata.labels` and `metadata.annotations` |
| Connector `generation` | `metadata.generation` |
| Desired configuration | `spec` |
| Observed lifecycle state | `status` |

Connector `definition` maps only to `spec.definition`; connector identity,
labels, annotations, generation, and release state are never mixed into that
document. Connector definitions are sensitive because they may contain OAuth
client secrets. Terraform still stores sensitive values in state, so use an
encrypted state backend with appropriately restricted access. When the API
redacts a connector secret on read, the provider retains the prior state value
at that field and continues detecting drift in non-secret fields.

Managed key material and actor signing material are write-only API values and
are not exposed by this provider's HCL schema or copied into Terraform state.

## Building

```bash
cd terraform/provider
go build -o terraform-provider-authproxy
```

## Development Testing

Add a dev override to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/rmorlok/authproxy" = "/path/to/terraform/provider"
  }
  direct {}
}
```
