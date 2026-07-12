# Connector Resource Schema

This package defines connector resource configuration: auth methods, setup flow, probes, rate-limiting behavior, telemetry settings, labels, and connector metadata.

Use this package for connector shapes that are shared between config, core logic, and future API DTOs. API-specific request/response wrappers belong in `internal/schema/api`.

Connector-author setup-flow guidance lives in [`docs/src/content/docs/integration/connector-setup-flow.md`](../../../../docs/src/content/docs/integration/connector-setup-flow.md). Shared predicate behavior for setup steps, OAuth scopes, and probes lives in [`docs/src/content/docs/integration/connector-predicates.md`](../../../../docs/src/content/docs/integration/connector-predicates.md).
