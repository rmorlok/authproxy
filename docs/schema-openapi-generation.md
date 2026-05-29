# Schema And OpenAPI Generation

AuthProxy currently has two related contract artifacts:

- JSON Schema files under `internal/schema/...`, primarily for configuration and reusable resource contracts.
- Generated Swagger/OpenAPI docs under `internal/service/{api,admin_api}/swagger`, produced from route annotations and Go DTO structs.

This document evaluates whether to keep the current [`swaggo/swag`](https://github.com/swaggo/swag) workflow with centralized Go DTOs, or move toward JSON Schema-driven OpenAPI generation so JSON Schema and generated API docs cannot drift.

## Current State

`scripts/generate-swagger.sh` runs `swag init` twice: once for the public API definition in `internal/service/api/swagger/definition.go`, and once for the admin API definition in `internal/service/admin_api/swagger/definition.go`. Both runs parse the whole module with `--parseDependency` and `--parseInternal`, then normalize the generated filenames to `docs.go`, `docs.json`, and `docs.yaml`.

The generated docs are Swagger/OpenAPI 2.0 today. Route comments still own operation-level metadata such as paths, HTTP methods, status codes, parameters, response codes, and security annotations. Go DTO structs own most request and response body shapes.

The schema layout work has already reduced the historical drift surface:

- Public API request and response DTOs live under `internal/schema/api`.
- REST-managed reusable resources live under `internal/schema/resources/...`.
- Swagger-specific adapter DTOs live under `internal/schema/api/openapi`.
- `scripts/check-schema-layout.sh` prevents resource packages from importing API DTOs, route-local `*RequestJson` and `*ResponseJson` structs, and route-local `Swagger*` structs.
- `./scripts/preflight.sh` regenerates Swagger docs and fails if generated files drift from committed output.

The remaining unavoidable split is that route annotations describe operations, while DTO structs and JSON Schema files describe reusable data contracts.

## Option 1: Keep swaggo/swag With Centralized DTOs

This keeps the current generation model and continues tightening the layout rules around it.

Drift prevention:

- Strong for REST DTOs used directly by handlers and Swagger annotations, because the same Go structs are used for runtime binding, rendering, and documentation.
- Good for generated docs, because preflight regenerates Swagger and checks the committed generated files.
- Partial for JSON Schema, because JSON Schema files and Go DTOs can still diverge unless package tests validate both intentionally.

Generated-doc stability:

- High. The repository already serves Swagger UI from the generated `docs.go` packages, and consumers already see Swagger 2.0 output.
- Changes stay incremental: move DTOs, add adapters only when `swag` cannot express a runtime type clearly, regenerate docs, and review the diff.

Tooling support:

- Good for the current Go route-annotation workflow.
- Limited to Swagger/OpenAPI 2.0 output, which misses modern OpenAPI 3.1 features and does not use full JSON Schema 2020-12 semantics.

Migration cost:

- Low. The main work is continuing the existing cleanup pattern and enforcing it in preflight.

Primary downside:

- JSON Schema is not the source of truth for REST body schemas. The project must rely on package tests, layout checks, and generated-doc diffs to prevent drift.

## Option 2: Generate Or Compose OpenAPI From JSON Schema

This would promote JSON Schema artifacts to the source of truth for request and response body schemas, then generate or compose an OpenAPI document from those schemas plus operation metadata.

Drift prevention:

- Strongest for body schemas if the OpenAPI document references or bundles the exact JSON Schema artifacts used elsewhere.
- Still incomplete for operations unless the project also chooses a source of truth for paths, parameters, auth requirements, status codes, examples, and error responses.

Generated-doc stability:

- Initially lower. AuthProxy would need a new OpenAPI 3.1 document root, bundling strategy, validation step, generated artifacts, and serving path.
- Existing Swagger UI setup and any consumers expecting Swagger 2.0 would need compatibility review.

Tooling support:

- OpenAPI 3.1 aligns much more closely with JSON Schema 2020-12 than Swagger 2.0 does. The [OpenAPI Specification](https://swagger.io/specification/) and JSON Schema community guidance both support this direction.
- The practical toolchain would still need to be selected and proved out. For example, bundlers such as [Redocly CLI](https://redocly.com/docs/cli/commands/bundle) can compose OpenAPI documents, but they do not remove the need to author operation metadata somewhere.

Migration cost:

- High. This is not just a generator swap; it is a docs architecture change.
- The project would need to define how JSON Schema IDs map into OpenAPI components, how relative `$ref` paths are bundled, how Swagger UI or a replacement viewer is served, and how CI catches incompatible changes.

Primary downside:

- The body-schema drift win is real, but the current project still has to maintain operation metadata outside JSON Schema. Without a larger OpenAPI-authoring strategy, this introduces another source of truth rather than removing one.

## Option 3: Hybrid Post-Processing

This would keep `swaggo/swag` for operation discovery, then post-process the generated Swagger/OpenAPI output to replace selected body schemas with JSON Schema-derived components.

Drift prevention:

- Better for selected bodies, but only where mappings are explicitly maintained.
- Risky for projection or adapter DTOs, where the public OpenAPI shape is intentionally simpler than the runtime/resource type.

Generated-doc stability:

- Medium to low. Post-processing can make generated diffs harder to review because the output no longer directly reflects either `swag` or the JSON Schema files.

Tooling support:

- Feasible, but custom. AuthProxy would own mapping rules between Go annotation type names, generated Swagger definitions, JSON Schema IDs, and bundled components.

Migration cost:

- Medium. Less work than a full OpenAPI 3.1 rewrite, but enough custom glue that it should solve a concrete consumer problem before being adopted.

Primary downside:

- It creates a bespoke generation pipeline while still keeping both route annotations and JSON Schema files.

## Recommendation

Keep `swaggo/swag` with centralized Go DTOs for now.

The recent schema layout work has changed the tradeoff: the highest-risk drift was route-local and Swagger-only DTO sprawl, and that is now constrained by package ownership rules plus preflight checks. Moving to JSON Schema-driven OpenAPI would improve body-schema reuse, but it would also require an OpenAPI 3.1 migration, a new operation metadata source, a bundling strategy, and compatibility work for docs serving and downstream consumers.

The recommended near-term posture is:

- Continue placing REST request and response DTOs in `internal/schema/api`.
- Continue placing reusable REST-managed resource models in `internal/schema/resources/...`.
- Continue using `internal/schema/api/openapi` for thin Swagger adapter DTOs when runtime types are too rich or recursive for stable generated docs.
- Keep `./scripts/preflight.sh` as the enforcement point for regenerated docs and schema package layout.
- Revisit OpenAPI 3.1 only when a consumer needs it, generated clients require it, or JSON Schema/OpenAPI body drift becomes a repeated maintenance problem despite the current guardrails.

No follow-up implementation issues are required from this evaluation because the recommendation is to keep the current tooling. If that recommendation changes later, create follow-up issues for:

- Defining an OpenAPI 3.1 root document and operation metadata source.
- Bundling AuthProxy JSON Schema files into OpenAPI components.
- Replacing or adapting Swagger UI/docs serving.
- Replacing the Swagger generation/preflight checks.
- Validating compatibility for any generated clients or docs consumers.

## References

- [`swaggo/swag`](https://github.com/swaggo/swag)
- [OpenAPI Specification](https://swagger.io/specification/)
- [JSON Schema and OpenAPI guidance](https://json-schema.org/blog/posts/validating-openapi-and-json-schema)
- [Redocly CLI bundle command](https://redocly.com/docs/cli/commands/bundle)
