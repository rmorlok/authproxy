# authproxy_connector

Manages an AuthProxy connector with automatic generation lifecycle.

## Example Usage

```hcl
resource "authproxy_connector" "gmail" {
  namespace = "root.production"

  definition = jsonencode({
    displayName = "Gmail"
    description  = "Google Gmail integration"
    auth = {
      type         = "OAuth2"
      clientId     = "your-client-id"
      clientSecret = "your-client-secret"
      authorization = {
        endpoint = "https://accounts.google.com/o/oauth2/v2/auth"
      }
      token = {
        endpoint = "https://oauth2.googleapis.com/token"
      }
      scopes = [{
        id     = "https://www.googleapis.com/auth/gmail.readonly"
        reason = "Read Gmail messages"
      }]
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
    displayName = "Gmail (Staging)"
    description  = "Gmail connector under review"
    auth = {
      type = "no-auth"
    }
  })
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace this connector belongs to.
- `definition` - (Required, Sensitive) The connector provider definition as JSON. Use `jsonencode()` for readable HCL. This maps exclusively to API `spec.definition`; resource metadata and release state are separate. Because definitions can contain credentials, secure access to Terraform state.
- `labels` - (Optional) A map of labels.
- `annotations` - (Optional) A map of annotations.
- `publish` - (Optional, default `true`) Whether to publish newly created or currently managed draft generations. Changing it from true to false does not demote an already published generation; it controls what happens when the next definition change creates a generation.

## Attribute Reference

- `id` - The stable connector ID (persists across generation changes).
- `generation` - The selected connector generation (`metadata.generation`).
- `state` - The current generation state (`draft`, `primary`, `active`, `archived`).
- `display_name` - Display name extracted from the definition.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Generation Lifecycle

The connector resource abstracts generation management:

- **Create**: Creates generation 1 with desired release state `primary` when `publish = true`, or `draft` when false.
- **Update a published definition**: Creates a new generation. With `publish = true`, the new generation becomes primary and the previous primary becomes active.
- **Update a draft definition**: Updates that draft generation in place.
- **Update labels or annotations**: Updates resource metadata without changing a published generation.
- **Update `publish` from false to true**: Promotes the managed draft generation to primary.
- **Destroy**: Archives all non-archived generations.

The API redacts connector credentials unless the caller can replay secrets. On a redacted read, the provider preserves prior values only at masked fields so credentials remain stable while non-secret drift is still detected.

## Import

Connectors can be imported by ID:

```bash
terraform import authproxy_connector.example cxr_abc123
```
