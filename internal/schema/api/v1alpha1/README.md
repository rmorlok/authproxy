# API v1alpha1 Transport Schema

This package owns API-version-specific list and action transport composition.
Managed resource objects, metadata, references, and conditions remain under
`internal/schema/resources`; pagination and imperative action envelopes do not.

Resource packages must not import this package. Route and API schema packages
compose resource-owned item/spec/status types with `ResourceList` and `Action`.
