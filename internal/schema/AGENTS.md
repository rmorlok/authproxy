# AGENTS.md

This directory owns AuthProxy's public and internal data contracts. Keep schema packages narrowly scoped and preserve dependency direction.

## Package Layout

- `internal/schema/common` contains shared primitives used by multiple schemas, such as string values, images, durations, byte sizes, request types, and raw JSON helpers.
- `internal/schema/config` contains YAML/JSON config-file syntax. It may reference common primitives, auth permissions, and resource definitions used in config.
- `internal/schema/auth` contains auth/JWT-specific schema such as permissions. Namespace path and matcher helpers live in `internal/schema/resources/namespace`; the auth package only re-exports namespace helpers for compatibility.
- `internal/schema/resources/...` contains reusable resource definition models. Use this tree for system resources such as connectors, namespaces, and rate-limit definitions; do not put API envelopes, pagination responses, or dry-run request/response bodies here.
- `internal/schema/api` contains API request/response DTOs. Route handlers should import these instead of defining local wire structs. Resource packages must not import it.

## Conventions

- Every schema package with Go contract types should have a `schema.go`, `schema.json`, tests that compile/validate the JSON schema, and a README explaining what belongs there.
- Go structs that represent serialized contracts should carry both `json` and `yaml` tags unless a field is deliberately not serialized.
- Prefer moving shared/resource contract types into `resources/...` or `common` instead of defining route-local DTOs in `internal/routes`.
- Keep JSON schema `$id` values aligned with the package path under `schema/...`.
- When moving a contract package, update JSON-schema `$ref` paths, schema tests, `internal/schema/reexport.go`, and any package README/AGENTS guidance in the same change.
