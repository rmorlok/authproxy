# Schema Packages

`internal/schema` contains AuthProxy's serialized contract types and their JSON Schema definitions.

- `common`: shared primitive value types.
- `config`: application configuration file syntax.
- `auth`: authentication and authorization contract types.
- `resources`: REST-managed resource models.
- `api`: API request/response DTOs, added as the API contract consolidation continues.

Resource packages are intentionally separate from API DTOs. API models can compose resources, but resources must not depend on API-specific request or response wrappers.
