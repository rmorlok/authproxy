# authproxy_namespace (Data Source)

Reads an existing AuthProxy namespace.

## Example Usage

```hcl
data "authproxy_namespace" "production" {
  path = "root.production"
}
```

## Argument Reference

- `path` - (Required) The namespace path.

## Attribute Reference

- `state` - The namespace state.
- `key_id` - The key ID, if set.
- `labels` - Labels map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
