# authproxy_actor

Manages an AuthProxy actor. Actors represent users or entities that own connections to external services.

## Example Usage

```hcl
resource "authproxy_actor" "service_account" {
  namespace   = "root.production"
  external_id = "svc-data-pipeline"
  labels = {
    role = "service-account"
    team = "data"
  }
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace this actor belongs to.
- `external_id` - (Required, ForceNew) The external identifier for this actor.
- `labels` - (Optional) A map of labels.

## Attribute Reference

- `id` - The actor ID.
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Import

Actors can be imported by ID:

```bash
terraform import authproxy_actor.example act_abc123
```
