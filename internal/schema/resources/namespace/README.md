# Namespace Resource Schema

This package owns the `authproxy.net/v1alpha1` `Namespace` resource, namespace
path and matcher validation, and constants for namespace hierarchy behavior.

Use this package for namespace resource semantics. Auth permissions reference namespace matchers, but namespace validation itself is not auth-specific.

API list and action transports for namespace routes live in
`internal/schema/api`; the durable resource itself lives here so API,
configuration, core, and persistence conversions share one contract.

A namespace's read-only resource name is set automatically to the final segment
of its path when the namespace is created. For example, `root.prod.billing` has
the name `billing`. Namespace paths are immutable, so namespace names cannot be
changed after creation.

The resource maps hierarchy and identity as follows:

- `metadata.id` is the immutable canonical path, such as `root.prod.billing`.
- `metadata.name` is the final path segment, such as `billing`.
- `metadata.namespace` is the parent path, such as `root.prod`; it is omitted
  only for the `root` resource.
- `spec.encryptionKeyRef` is the optional desired encryption-key selection.
- `status.state` is the server-owned lifecycle observation.

`PathFromMetadata` and `NewResourceMetadata` are the canonical conversion
helpers. Persistence remains flat and converts at the core boundary rather
than leaking database rows into the public resource schema.

Updates use `NamespacePatch`, whose pointer-based metadata fields preserve
the difference between an omitted map and an explicitly empty map. Use
`NamespacePatch.ApplyTo` to apply a patch and enforce immutable namespace
identity; route code should not reproduce that merge logic.
