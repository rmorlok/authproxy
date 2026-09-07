# authproxy_connector (Data Source)

Reads an existing AuthProxy connector.

## Example Usage

```hcl
data "authproxy_connector" "gmail" {
  id = "cxr_abc123"
}
```

## Argument Reference

- `id` - (Required) The connector ID.

## Attribute Reference

- `namespace` - The namespace.
- `version` - The selected connector generation (`metadata.generation`).
- `state` - The current version state.
- `display_name` - The display name.
- `description` - The description.
- `logo` - The public URL or data URL from `spec.definition.logo`.
- `labels` - Labels map.
- `annotations` - Annotations map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
