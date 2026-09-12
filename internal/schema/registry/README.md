# Resource Registry

`NewResourceScheme()` combines generic manifest decoding with the shared
capabilities of every canonical AuthProxy resource. The catalogue in
`resourceTypes()` registers each resource and typed list together with its patch
factory, metadata accessor, ID validator, lifecycle validation and patch
application bindings, and REST collection name.

Use `scheme.Lookup(gvk)` or `LookupResource(gvk)` to obtain a descriptor.
`TypeOf(resource)` identifies a concrete resource without a caller-side type
switch. Descriptors are fresh values; metadata access returns a detached copy.
Wrong concrete types and typed nil pointers return errors.

`scheme.DecodeJSON` and `scheme.DecodeYAML` decode resources and typed lists.
`descriptor.DecodePatchJSON` explicitly selects and validates the patch contract,
which shares the resource's GVK. `descriptor.ApplyPatch` delegates to the
resource package's immutable-field and merge rules and returns the resulting
resource without modifying the original. Lifecycle validation is available
through `descriptor.ValidateResource` and stays implemented in resource packages.

Each constructor returns an independent generic scheme. Additional custom
contracts can be registered on its embedded `Scheme`; this does not grant them
canonical resource capabilities. There is no mutable global registry or `init()`
registration. Actions, projections, and heterogeneous `List` input remain
consumer-specific contracts.

Application policy remains outside this package: which resources an operation
supports, namespace defaults, authorization and endpoint selection, HTTP calls,
reconciliation, dependency ordering, and execution. The generic
`internal/schema/manifest` package remains independent of concrete resource types.

This package composes resource types and API list envelopes from outside both
packages, preserving schema dependency direction. It defines capabilities, not
new serialized contracts, and therefore needs no independent JSON Schema.
