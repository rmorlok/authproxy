# Resource Registry

`NewResourceScheme()` is the central registration list for AuthProxy's canonical
resources and typed resource-list envelopes. Each call returns an independent
`manifest.Scheme`; there is no mutable global scheme or `init()` registration.

Add new resources here once, using `registerResource[T]` to register the resource
and its `api/v1alpha1.ResourceList[T]` envelope together. Application consumers
such as `internal/apply` use this constructor rather than maintaining their own
GVK-to-Go-type registrations. The generic decoding machinery remains in
`internal/schema/manifest`.

Recognition is separate from operation support. Consumers still decide which
resources they can apply, create, or update, and perform lifecycle validation.
Typed API handlers should keep decoding their specific request contracts:
resource patches share a resource's GVK and cannot be distinguished by GVK
alone. Actions, projections, and heterogeneous `List` input are not canonical
resource registrations; consumers handle those separately.

This package composes resource types and API list envelopes from outside both
packages, preserving schema dependency direction. It defines no new serialized
contracts and therefore needs no independent JSON Schema.
