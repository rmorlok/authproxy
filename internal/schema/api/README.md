# API Schema

This package contains API request and response DTOs. These types describe the wire contract at AuthProxy route boundaries.

API models may compose shared primitives from `internal/schema/common` and resource models from `internal/schema/resources/...`. Resource packages must not import this package.

OpenAPI-only generator adapters live under `internal/schema/api/openapi`. Keep those adapters thin: they may compose API DTOs for documentation, but runtime route request/response contracts should remain in this package.
