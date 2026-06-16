# authproxy_key (Data Source)

Reads an existing AuthProxy key.

## Example Usage

```hcl
data "authproxy_key" "main" {
  id = "key_abc123"
}
```

## Argument Reference

- `id` - (Required) The key ID.

## Attribute Reference

- `namespace` - The namespace.
- `state` - The current state.
- `labels` - Labels map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
