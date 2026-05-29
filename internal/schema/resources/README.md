# Resource Schemas

This tree contains schema packages for resources managed by AuthProxy in a RESTful style.

Resource packages can depend on `internal/schema/common` and on other resource packages when there is a genuine resource relationship. They must not import `internal/schema/api`; API request and response wrappers compose resources from the outside.

Preflight runs `scripts/check-schema-layout.sh`, which fails if a resource package imports `internal/schema/api`. If an API response needs to expose a resource, add the envelope or request/response wrapper in `internal/schema/api` and reference the resource package from there.
