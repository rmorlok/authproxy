# authproxy_encryption_key (Data Source)

Reads an existing AuthProxy encryption key.

## Example Usage

```hcl
data "authproxy_encryption_key" "main" {
  id = "ek_abc123"
}
```

## Argument Reference

- `id` - (Required) The encryption key ID.

## Attribute Reference

- `namespace` - The namespace.
- `state` - The current state.
- `labels` - Labels map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
