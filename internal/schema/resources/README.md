# Resource Schemas

This tree contains schema packages for resources managed by AuthProxy in a RESTful style.

Resource packages can depend on `internal/schema/common` and on other resource packages when there is a genuine resource relationship. They must not import `internal/schema/api`. Routes use canonical resources and patches directly when those are the complete wire body; endpoint-specific API wrappers compose resources from the outside.

Preflight runs `scripts/check-schema-layout.sh`, which fails if a resource package imports `internal/schema/api`. Add an envelope or request/response DTO in `internal/schema/api` only when the endpoint contract contains more than the canonical resource or patch.
