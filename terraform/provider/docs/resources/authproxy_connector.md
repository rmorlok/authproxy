# authproxy_connector

Manages an AuthProxy connector with automatic version lifecycle. When the definition changes, a new version is created and optionally promoted to primary.

## Example Usage

```hcl
resource "authproxy_connector" "gmail" {
  namespace = "root.production"

  definition = jsonencode({
    display_name = "Gmail"
    description  = "Google Gmail integration"
    auth = {
      type          = "oauth2"
      client_id     = "your-client-id"
      client_secret = "your-client-secret"
      auth_url      = "https://accounts.google.com/o/oauth2/v2/auth"
      token_url     = "https://oauth2.googleapis.com/token"
      scopes        = ["https://www.googleapis.com/auth/gmail.readonly"]
    }
  })

  labels = {
    service = "google"
    type    = "email"
  }
}
```

### Draft Connector (staging/review)

```hcl
resource "authproxy_connector" "gmail_staging" {
  namespace = "root.staging"
  publish   = false

  definition = jsonencode({
    display_name = "Gmail (Staging)"
    description  = "Gmail connector under review"
    auth = {
      type = "no_auth"
    }
  })
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace this connector belongs to.
- `definition` - (Required) The connector definition as JSON. Use `jsonencode()` for readable HCL. Includes auth configuration, probes, rate limiting, etc.
- `labels` - (Optional) A map of labels.
- `publish` - (Optional, default `true`) Whether to promote new versions to primary state. When `false`, versions remain in draft state.

## Attribute Reference

- `id` - The stable connector ID (persists across version changes).
- `version` - The current version number.
- `state` - The current version state (`draft`, `primary`, `active`, `archived`).
- `display_name` - Display name extracted from the definition.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Version Lifecycle

The connector resource abstracts version management:

- **Create**: Creates version 1. If `publish = true`, promotes to `primary`.
- **Update (definition changed)**: Creates a new version. If `publish = true`, promotes to `primary` (previous primary becomes `active`).
- **Update (labels only)**: Updates labels on the current version without creating a new one.
- **Update (publish false -> true)**: Promotes the current draft version to `primary`.
- **Destroy**: Archives all non-archived versions.

## Import

Connectors can be imported by ID:

```bash
terraform import authproxy_connector.example cxr_abc123
```
