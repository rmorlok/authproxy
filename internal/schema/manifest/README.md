# Manifest Scheme

This package owns strict JSON/YAML decoding and GVK dispatch for AuthProxy
manifests. Callers register resource, list, projection, or action contract
types; the scheme reads `apiVersion` and `kind`, selects a fresh typed value,
then rejects unknown fields while decoding it.

Multi-document support is intentionally YAML-only. A JSON list is itself a
registered `<Kind>List` object rather than an untyped array of manifests. This
package owns decoding infrastructure, not a serialized contract, so it has no
independent JSON Schema.
