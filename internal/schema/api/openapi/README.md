# API OpenAPI Schema

This package contains OpenAPI-only helper models for API DTOs when the Swagger generator needs a documentation shape that differs from the runtime Go type layout.

Keep runtime wire DTOs in `internal/schema/api`. Use this package only for generator adapters that compose those DTOs without duplicating their fields.
