# API OpenAPI Schema

This package contains OpenAPI-only helper models for API DTOs when the Swagger generator needs a documentation shape that differs from the runtime Go type layout.

Keep runtime wire DTOs in `internal/schema/api`. Use this package only for generator adapters that compose those DTOs without duplicating their fields.

These types exist only to work around generator limitations, such as making recursive or schema-heavy fields opaque to swaggo. Name them after the API DTO they document and keep field lists intentionally minimal so they do not become a second source of truth.
