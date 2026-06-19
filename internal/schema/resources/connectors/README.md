# Connector Resource Schema

This package defines connector resource configuration: auth methods, setup flow, probes, rate-limiting behavior, telemetry settings, labels, and connector metadata.

Use this package for connector shapes that are shared between config, core logic, and future API DTOs. API-specific request/response wrappers belong in `internal/schema/api`.

Connector-author setup-flow guidance lives in [`docs/connector-setup-flow.md`](../../../../docs/connector-setup-flow.md). Shared predicate behavior for setup steps, OAuth scopes, and probes lives in [`docs/connector-predicates.md`](../../../../docs/connector-predicates.md).
