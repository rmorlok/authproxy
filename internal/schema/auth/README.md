# Auth Schema

This package contains authentication and authorization contract types, including permissions used in JWTs and configuration.

Namespace path and matcher semantics are owned by `internal/schema/resources/namespace`. This package re-exports namespace helpers for compatibility while callers migrate to the resource package.

Permission namespaces may be literal namespace matchers such as `root.team.**`, or actor-templated matchers using `{{external_id}}`, `{{labels.<label>}}`, and `{{annotations.<annotation>}}`. Missing label or annotation values render the permission unmatched rather than broadening access.
