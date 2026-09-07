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
- `state` - (Optional) The desired key state (`spec.desiredState`). The computed value reflects observed `status.state`.
- `labels` - (Optional) A map of labels.
- `annotations` - (Optional) A map of annotations.

## Attribute Reference

- `id` - The key ID.
- `state` - The current state.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

The provider creates keys with server-managed random key material. Provider
configuration and key material are write-only and are never stored in
Terraform state.

## Import

Keys can be imported by ID:

```bash
terraform import authproxy_key.example key_abc123
```
