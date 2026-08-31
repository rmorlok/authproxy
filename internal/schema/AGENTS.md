# AGENTS.md

This directory owns AuthProxy's public and internal data contracts. Keep schema packages narrowly scoped and preserve dependency direction.

## Package Layout

- `internal/schema/common` contains shared primitives used by multiple schemas, such as string values, images, durations, byte sizes, request types, and raw JSON helpers.
- `internal/schema/config` contains YAML/JSON config-file syntax. It may reference common primitives, auth permissions, and resource definitions used in config.
- `internal/schema/auth` contains auth/JWT-specific schema such as permissions. Namespace path and matcher helpers live in `internal/schema/resources/namespace`; the auth package only re-exports namespace helpers for compatibility.
- `internal/schema/resources/...` contains canonical serialized resource models and their resource-specific patches. Use this tree for system resources such as connectors, namespaces, and rate-limit definitions; do not put endpoint-only envelopes, pagination responses, or dry-run request/response bodies here.
- `internal/schema/api` contains endpoint-specific request/response DTOs and shared API transports that are not themselves canonical resources. Route handlers should import these instead of defining local endpoint-only wire structs. Resource packages must not import it.
- `internal/schema/api/openapi` contains documentation-only projections needed by the OpenAPI generator. It is downstream of the runtime contracts: packages outside `openapi` must not use these projections for request binding, response rendering, validation, or business logic.

## Conventions

- Every schema package with Go contract types should have a `schema.go`, `schema.json`, tests that compile/validate the JSON schema, and a README explaining what belongs there.
- Go structs that represent serialized contracts should carry both `json` and `yaml` tags unless a field is deliberately not serialized.
- Prefer moving shared/resource contract types into `resources/...` or `common` instead of defining route-local DTOs in `internal/routes`.
- When an endpoint body is exactly a canonical resource or resource patch, routes should use that type directly. Do not add an alias in `internal/schema/api` merely to give the resource an endpoint-specific name.
- Endpoint-specific DTOs that differ from canonical resources belong in `internal/schema/api`; route packages may convert to/from runtime models but should not own those serialized contracts.
- A type alias is not an API boundary. If an endpoint needs an independently evolving contract, define a distinct type. If it does not, use the canonical resource, patch, or shared transport directly.
- Swagger-only adapter models belong in `internal/schema/api/openapi`, not in `internal/routes`.
- Keep JSON schema `$id` values aligned with the package path under `schema/...`.
- When moving a contract package, update JSON-schema `$ref` paths, schema tests, and any package README/AGENTS guidance in the same change.
