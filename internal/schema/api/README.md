# API Schema

This package contains API request and response DTOs. These types describe the wire contract at AuthProxy route boundaries.

API models may compose shared primitives from `internal/schema/common` and resource models from `internal/schema/resources/...`. Resource packages must not import this package.

Rate-limit API envelopes, pagination responses, and dry-run DTOs live here while the reusable rate-limit definition remains in `internal/schema/resources/rate_limit`. Encryption-key API DTOs live here too; key material syntax composes `internal/schema/config.KeyData`, and the API exposes its own state enum rather than database storage types.

OpenAPI-only generator adapters live under `internal/schema/api/openapi`. Keep those adapters thin: they may compose API DTOs for documentation, but runtime route request/response contracts should remain in this package.
