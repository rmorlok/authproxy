# authproxy_key

Manages an AuthProxy key used to encrypt sensitive data like OAuth tokens.

## Example Usage

```hcl
resource "authproxy_key" "main" {
  namespace = "root.production"
  labels = {
    purpose = "token-encryption"
  }
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace this key belongs to.
- `state` - (Optional) The desired state of the key.
- `labels` - (Optional) A map of labels.

## Attribute Reference

- `id` - The key ID.
- `state` - The current state.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Import

Keys can be imported by ID:

```bash
terraform import authproxy_key.example key_abc123
```
