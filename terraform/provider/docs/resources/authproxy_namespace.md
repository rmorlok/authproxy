# authproxy_namespace

Manages an AuthProxy namespace. Namespaces provide logical grouping for connectors, connections, and other resources.

## Example Usage

```hcl
resource "authproxy_namespace" "production" {
  path = "root.production"
  labels = {
    env  = "production"
    team = "platform"
  }
}
```

## Argument Reference

- `path` - (Required, ForceNew) The namespace path. Must start with "root".
- `labels` - (Optional) A map of labels for the namespace.

## Attribute Reference

- `state` - The namespace state.
- `encryption_key_id` - The ID of the encryption key associated with this namespace.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Import

Namespaces can be imported by path:

```bash
terraform import authproxy_namespace.example root.my-namespace
```
