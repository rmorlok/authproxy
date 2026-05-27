# AuthProxy Documentation

This directory holds long-form documentation that supplements the [main repo README](../README.md). The main README has the conceptual overview and getting-started material; the pages here go deep on specific features and operational topics.

## Feature guides

- **[CLI (`ap`)](cli.md)** — config file shape (`~/.authproxy.yaml`), RS256 vs HS256 signing keys, every command (list, sign-jwt / verify-jwt, signing-proxy, the connection-scoped streaming `proxy` with `curl`/`wget` modes, marketplace helpers).
- **[Rate limits](rate-limits.md)** — define rate-limit resources, configure connector-level reactive 429 handling, understand the request log attribution fields.
- **[Labels and annotations](labels.md)** — the Kubernetes-style label system, system labels under `apxy/`, carry-forward through the namespace → connector → connection hierarchy, label selectors, per-request label snapshots.

## Operational guides

- **[Telemetry](telemetry.md)** — OpenTelemetry traces / metrics / logs across all services, OTLP exporter configuration, label projection, the metrics catalog, and the local Grafana + Tempo + Loki + Prometheus dev stack under `docker compose --profile observability`.
- **[Background tasks](background_tasks.md)** — running the worker, monitoring queues with Asynqmon.
- **[Blob storage](blob_storage.md)** — viewing data stored in MinIO / S3 (full request bodies, etc.).
- **[Redis insight](redis_insight.md)** — viewing data stored in Redis (rate-limit counters, session state, etc.).

## Reference

- **[OAuth test provider gaps](oauth_test_provider_gaps.md)** — limitations of the in-repo OAuth test provider that the integration tests run against.
- **[Schema and OpenAPI generation](schema-openapi-generation.md)** — recommendation on keeping `swaggo/swag` vs. moving toward JSON Schema-driven OpenAPI generation.

## Architecture diagrams

- **[OAuth connection flow](oauth_flow.mmd)** — Mermaid diagram of the OAuth2 authorization-code lifecycle.
- **[Marketplace portal session flow](marketplace_portal.mmd)** — Mermaid diagram of the embeddable marketplace's session handshake.

## Where else docs live

- **[Main README](../README.md)** — overview, core concepts, running locally, testing, UI, CLI.
- **[RELATED.md](../RELATED.md)** — related products in the API integration space.
- **[Terraform provider docs](../terraform/provider/docs/resources/)** — generated reference for each `authproxy_*` Terraform resource.
- **[Terraform examples](../terraform/provider/examples/resources/)** — runnable HCL examples per resource.
