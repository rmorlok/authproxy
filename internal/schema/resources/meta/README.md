# Resource Metadata Schema

This package owns the schema-version, kind, identity, labels, annotations,
references, and conditions shared by AuthProxy resource contracts. It also
provides lifecycle-aware metadata validation/defaulting, merge-patch helpers,
and canonical spec hashing.

Resource packages embed these definitions and add only their own kind, spec,
status, and business-neutral resource validation. API list and action
transports belong in `internal/schema/api/v1alpha1`; GVK decoding belongs in
`internal/schema/manifest`. This package must not import either API package.

Database rows remain persistence models. Conversion code may use this package,
but public resource structs should not become database models.

The JSON Schema exposes an open `Resource` base for composition. A concrete
resource schema adds its kind/spec/status definitions with `allOf`, then sets
`unevaluatedProperties: false`; this reuses TypeMeta without repeating fields
or allowing unknown properties at the concrete boundary.
