# authproxy_encryption_key

Manages an AuthProxy encryption key used to encrypt sensitive data like OAuth tokens.

## Example Usage

```hcl
resource "authproxy_encryption_key" "main" {
  namespace = "root.production"
  labels = {
    purpose = "token-encryption"
  }
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace this encryption key belongs to.
- `state` - (Optional) The desired state of the encryption key.
- `labels` - (Optional) A map of labels.

## Attribute Reference

- `id` - The encryption key ID.
- `state` - The current state.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Import

Encryption keys can be imported by ID:

```bash
terraform import authproxy_encryption_key.example ek_abc123
```
