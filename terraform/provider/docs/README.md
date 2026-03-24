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
- `authproxy_encryption_key` - Manages encryption keys
- `authproxy_actor` - Manages actors (users/entities that own connections)
- `authproxy_connector` - Manages connectors with automatic version lifecycle

## Data Sources

- `data.authproxy_namespace` - Reads an existing namespace
- `data.authproxy_encryption_key` - Reads an existing encryption key
- `data.authproxy_actor` - Reads an existing actor
- `data.authproxy_connector` - Reads an existing connector

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
