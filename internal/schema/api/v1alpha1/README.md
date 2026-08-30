# API v1alpha1 Transport Schema

This package owns API-version-specific list and action transport composition.
Managed resource objects, metadata, references, and conditions remain under
`internal/schema/resources`; pagination and imperative action envelopes do not.

Resource packages must not import this package. Route and API schema packages
compose resource-owned item/spec/status types with `ResourceList` and `Action`.

Managed list responses use `<Kind>List`, always contain an `items` array, and
put opaque pagination tokens in `metadata.continue`. API create and update
bodies are full resource objects validated with the corresponding metadata
lifecycle mode; status is rejected on writes. Typed action requests use
`ActionRequest` and likewise reject status, while action responses may include
server-observed status.

HTTP handlers use the lifecycle-aware helpers in `internal/apgin` so strict
JSON decoding, request-body preservation, secret-placeholder rejection,
response validation, and API redaction stay consistent. The concrete types in
`internal/schema/api/openapi` are documentation-only adapters for swaggo,
which cannot reliably render the generic list and action types.
