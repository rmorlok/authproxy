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
- `version` - The current version number.
- `state` - The current version state.
- `display_name` - The display name.
- `description` - The description.
- `logo` - The logo.
- `labels` - Labels map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
