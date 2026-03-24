# authproxy_actor (Data Source)

Reads an existing AuthProxy actor.

## Example Usage

```hcl
data "authproxy_actor" "existing" {
  id = "act_abc123"
}
```

## Argument Reference

- `id` - (Required) The actor ID.

## Attribute Reference

- `namespace` - The namespace.
- `external_id` - The external identifier.
- `labels` - Labels map.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.
