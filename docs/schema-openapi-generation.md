# Schema And OpenAPI Generation

AuthProxy currently has two related contract artifacts:

- JSON Schema files under `internal/schema/...`, used to validate serialized resource, config, auth, common, and API DTO contracts.
- Swagger/OpenAPI files under `internal/service/{api,admin_api}/swagger/`, generated from Go route annotations and Go DTO types.

This note evaluates whether OpenAPI should become JSON Schema-driven, or whether AuthProxy should keep the current `swaggo/swag` flow while centralizing DTO ownership.

## Current Generator

`scripts/generate-swagger.sh` runs `swag init` twice: once for the API service and once for the Admin API service. Both runs parse the whole repo with `--parseDependency` and `--parseInternal`, then move generated `docs.go`, `docs.json`, and `docs.yaml` into each service's swagger package.

`./scripts/preflight.sh` reruns that generator and fails if the committed Swagger files change. CI also has a Swagger workflow that fails PRs when generated docs are stale.

Important constraints:

- The generated documents are Swagger/OpenAPI 2.0 (`"swagger": "2.0"`), while the JSON Schema files are draft 2020-12.
- Route comments remain the source of truth for paths, parameters, response status codes, tags, and security annotations.
- Go DTO structs remain the source the generator can inspect for response and request body schemas.
- Some runtime Go types are not ideal documentation shapes, so `internal/schema/api/openapi` contains thin generator-only adapters.
- `swag` can be sensitive to dependency parsing and package/type layout; examples seen in this repo include same-package alias/list wrapper quirks and schema-resource internals that need OpenAPI adapters.

## Option 1: Keep `swaggo/swag` With Centralized DTOs

This is the current direction: move API wire DTOs into `internal/schema/api`, make routes alias those types, keep resource schemas under `internal/schema/resources/...`, and reserve `internal/schema/api/openapi` for generator-only adapters.

Benefits:

- Lowest migration cost because the Gin Swagger wiring, route comments, generated `docs.go`, and preflight workflow remain intact.
- Drift is reduced by using the same centralized Go DTO structs in routes and generated Swagger.
- JSON Schema tests already validate the schema artifacts independently.
- The remaining schema-project work can remove more duplicated route/swagger structs without changing the public docs pipeline.

Limitations:

- Swagger and JSON Schema are still generated from different sources, so exact schema drift is possible when a Go DTO and `schema.json` are edited inconsistently.
- The generated docs stay on OpenAPI 2.0 unless the generator stack changes.
- OpenAPI-only adapters are still needed where the generator cannot safely parse runtime/resource types.

Cost: low. This is mostly discipline, package ownership, and guardrail work.

## Option 2: Generate Or Compose OpenAPI From JSON Schema

OpenAPI 3.1 aligns its Schema Object with JSON Schema draft 2020-12, which is a much better theoretical fit for the `internal/schema/.../schema.json` files than Swagger 2.0. A future OpenAPI 3.1 document could use `components.schemas` that reference or inline AuthProxy's JSON Schema artifacts, then use a bundler such as Redocly CLI to produce a single published document.

Benefits:

- Strongest drift prevention for body schemas: the JSON Schema files could become the schema source for both validation and OpenAPI.
- Better support for current JSON Schema constructs such as `const`, conditional schemas, and draft 2020-12 references.
- It would make the schema package layout more valuable to API consumers, not only internal tests.

Limitations:

- It does not remove the need to describe operations. Paths, parameters, status codes, auth schemes, pagination, and examples still need a source of truth.
- AuthProxy would need a new OpenAPI 3.1 serving/generation path to replace the current `docs.go` registration flow.
- Some API response types are projections, not resource definitions. For example, `internal/schema/api` connector summary responses flatten resource fields and add runtime fields, while request `definition` fields correctly reference `internal/schema/resources/connectors/schema.json`.
- Tooling and downstream client support for OpenAPI 3.1 is improving but is still more uneven than OpenAPI 3.0/Swagger 2.0.

Cost: medium to high. This would be a new docs architecture, not a drop-in replacement for `swag init`.

## Option 3: Hybrid Post-Processing

A smaller bridge would keep `swaggo/swag` for paths and operation metadata, then post-process the generated Swagger output to replace selected definitions with schemas derived from `internal/schema`.

Benefits:

- Keeps route comments and the current docs serving path.
- Could reduce drift for selected body schemas.

Limitations:

- Swagger 2.0 schema objects are not JSON Schema draft 2020-12, so this requires lossy down-conversion.
- Post-processing generated docs adds another fragile build step.
- It can obscure which source owns a schema when a generated definition is partly Go-derived and partly patched.

Cost: medium. The risk is not worth it as a durable direction.

## Recommendation

Keep `swaggo/swag` for now, continue centralizing API DTOs under `internal/schema/api`, and keep JSON Schema artifacts as the validation/specification source beside those DTOs. Do not migrate to JSON Schema-driven OpenAPI until the remaining API DTO packages are centralized and the duplicated Swagger-only structs have been removed or isolated.

This recommendation is intentionally conservative:

- It keeps the current generated-doc stability and preflight behavior.
- It gives reviewers smaller PRs while the schema package boundaries are still settling.
- It avoids committing to OpenAPI 3.1 serving/tooling before AuthProxy has a complete centralized API contract inventory.

The next practical steps are already represented by open `project:schema` issues:

- #385: remove duplicated Swagger models and add schema layout guardrails.
- #387: migrate rate limit and encryption key API contracts.
- #388: migrate auxiliary API DTOs.

After those land, revisit OpenAPI 3.1 with a small proof of concept that:

1. Builds an OpenAPI 3.1 root document for a narrow set of endpoints.
2. References `internal/schema/api/schema.json` and resource schemas from `components.schemas`.
3. Bundles the document into a single artifact.
4. Validates that Swagger UI, generated clients, and repo preflight can consume the result without worse churn than the current generated files.

No new follow-up issue is needed from this evaluation because the recommendation is to keep the current generator in the near term and use the existing schema-project follow-ups to reduce drift.

## References

- [`swaggo/swag`](https://github.com/swaggo/swag) documents its current stable flow as converting Go annotations to Swagger 2.0.
- The [OpenAPI 3.1 specification](https://swagger.io/specification/) defines the Schema Object as a superset of JSON Schema draft 2020-12.
- JSON Schema's [OpenAPI validation guidance](https://json-schema.org/blog/posts/validating-openapi-and-json-schema) notes that OpenAPI 3.1 has a configurable JSON Schema dialect and that validator support for newer dynamic-reference behavior can vary.
- [Redocly CLI `bundle`](https://redocly.com/docs/cli/commands/bundle) can bundle a root OpenAPI document by following `$ref` references into a single JSON or YAML artifact.
