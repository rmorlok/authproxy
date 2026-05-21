# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

## Project Overview

AuthProxy is an open-source, embeddable integration platform-as-a-service (iPaaS). It manages the connection lifecycle to 3rd party systems, allowing applications to call those systems through an authenticating proxy.

## Workflow

### Preflight (required before commit)

```bash
./scripts/preflight.sh
```

This regenerates Swagger docs and checks the integration-tests module is consistent. Fix any failures before committing.

### Working with pull requests

- **Apply the issue's labels to the PR.** When opening a PR that closes a labelled issue, copy those labels onto the PR (e.g. `gh pr create --label "project:api-key"`). Keeps project-tracking views consistent and surfaces the PR in the same dashboards as the issue.
- **Tag the PR with the local clone it came from.** Several clones of this repo coexist on the same machine (`~/src/authproxy1`, `~/src/authproxy2`, …) and it's useful to know which one produced a given PR. Take the current directory's basename — if it matches `authproxy<N>` (single-digit suffix `0-9`) — and apply a `clone:authproxy<N>` label. Create the label first if it doesn't exist (`gh label create clone:authproxy<N> --color BFD4F2`). If the basename doesn't match the `authproxy<N>` pattern (e.g. a fork, a feature-named worktree), skip this label.
- **Respond to PR review comments after addressing them.** When you push a change that resolves a review comment, reply on the original comment thread describing what changed (link to the commit if useful). Don't leave the reviewer guessing whether their feedback landed.
- **Include screenshots in the PR description for UI changes.** Any PR that touches `ui/admin` or `ui/marketplace` should embed before/after (or empty/populated/edge-case) screenshots inline in the PR description so reviewers don't have to spin up a dev server to see what changed. Capture them via Chrome DevTools MCP — against a running dev server for page-level changes, or against Storybook (`yarn workspace @authproxy/admin storybook`) for component-level changes. **Prefer hosting the images outside the repo** — drag-and-dropping into the PR description on github.com produces `user-attachments` URLs that survive without polluting the repo. The fallback (when image hosting isn't available, e.g. for automated agents — GitHub doesn't expose `user-attachments` upload via REST) is to commit PNGs under `docs/screenshots/<branch-or-feature-name>/` and reference them from the PR description via `https://raw.githubusercontent.com/{owner}/{repo}/{branch}/docs/screenshots/...` URLs so they render in the description.

## Running locally

### Backend dependencies

```bash
# Data stores only (Postgres, Redis, MinIO, ClickHouse)
docker compose up -d

# Full stack including the AuthProxy server
docker compose --profile server up -d

# Tear down everything (containers + volumes)
./scripts/teardown-docker.sh
```

### Run the server

```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

The final arg is the service to run: `admin-api`, `api`, `public`, `worker`, or `all`.

### Run the client proxy

```bash
# JWT-signing reverse proxy to the AuthProxy server itself (dev tool).
go run ./cmd/cli signing-proxy --enableLoginRedirect=true --proxyTo=api

# Connection-scoped streaming reverse proxy through /_proxy_raw.
go run ./cmd/cli proxy --connection cxn_xxx --upstream-base https://api.openai.com

# One-shot through curl. Everything after `curl` is forwarded to curl
# verbatim; all ap proxy flags must come before it.
go run ./cmd/cli proxy --connection cxn_xxx curl https://api.openai.com/v1/models
```

### Other useful commands

```bash
# Print all routes
go run ./cmd/server routes --config=./dev_config/default.yaml
```

### Frontend

Node + yarn pinned via Volta (versions in `package.json`).

```bash
volta install node && volta install yarn
yarn install
yarn workspace @authproxy/marketplace dev
yarn workspace @authproxy/admin dev
```

### Monitoring tools

```bash
# RedisInsight — also available via docker-compose's "tools" profile (port 5540)
docker run -d --name redisinsight -p 5540:5540 -v redisinsight:/data --network authproxy redis/redisinsight:latest

# Asynqmon (background-task dashboard)
docker run --rm -d --name asynqmon --network authproxy -p 8090:8080 hibiken/asynqmon --redis-addr=redis-server:6379

# Asynq CLI
go install github.com/hibiken/asynq/tools/asynq@latest && asynq dash
```

## Architecture

### Service ports (defaults from `dev_config/default.yaml`)

| Service | Port | Role |
|---|---|---|
| `public` | 8080 | OAuth callbacks, marketplace |
| `api` | 8081 | Core API for application integration |
| `admin-api` | 8082 | Administrative API + UI |
| `worker` | 8083 (health) | Background-task processor (Asynq) |

All services are coordinated through the `cmd/server` entrypoint using the service-based architecture in `internal/service/`.

### Layering

- `internal/core` is the business-logic layer — fully hydrated models on top of the database and Redis.
- **Other packages should depend on `internal/core/iface` (interfaces) rather than `internal/core` directly.** This is the main layering rule that's easy to violate accidentally.
- Auth methods live under `internal/auth_methods/{oauth2,no_auth,…}` and import `internal/core/iface`, never `internal/core` directly (avoid cycles).

### Database

- Two providers supported: **SQLite** (default for dev) and **PostgreSQL**. Schemas must stay in sync.
- Per-package guide with deeper conventions: [`internal/database/AGENTS.md`](internal/database/AGENTS.md). Read it before touching migrations, models, or the DB interface.

### Auth

`internal/apauth/` is where authentication lives — `apauth/core` (request auth + actor types), `apauth/service` (validators, redirects), `apauth/jwt` (signing/verification), `apauth/tasks` (background work). Session JWTs are signed with keys configured under `system_auth.jwt_signing_key`.

### Background tasks

Asynq, fronted by `internal/apasynq` (testable interface) with API-exposed wrappers in `internal/tasks`. Tasks are registered through `service.RegisterTasks` and periodic tasks come from `service.GetCronTasks`.

### Configuration

YAML-based, loaded from `internal/schema/config`. Dev configs in `dev_config/`. Connector definitions support three auth types: **OAuth2**, **API Key** (placements: `bearer`, `header`, `query`, `basic`), and **NoAuth**. Connectors can declare **probes** to validate connection health.

### Other packages worth knowing

- `internal/apctx` — request context with correlation ids, an injectable clock, and value-applier helpers. Use `apctx.GetClock(ctx)` instead of `time.Now()` in any code under test.
- `internal/encrypt` + `internal/encfield` — AES-GCM encryption for `EncryptedField` columns. The re-encryption registry (`internal/database/reencrypt_registry.go`) drives key-rotation jobs over registered encrypted columns.
- `internal/request_log` — structured HTTP request/response logging with sensitive-data redaction.
- `internal/httpf` — HTTP client factory with mock support and OpenTelemetry instrumentation.
- `internal/aptelemetry` — bootstrap and shared helpers for OpenTelemetry (`New`, `NoopProviders`, `LabelProjector`). All services route through this package; no other code should construct OTel providers directly.

### Telemetry conventions

Full reference: [`docs/telemetry.md`](docs/telemetry.md). Day-to-day rules when adding instrumentation:

- **Config lives in one place.** The `telemetry:` block in `internal/schema/config` is the only knob — there is no per-package toggle. It is **off by default**, endpoint-gated (an empty `exporter.endpoint` falls through to no-op providers), and signals can be toggled independently under `telemetry.signals`.
- **Use the existing instrumented wrappers** rather than wiring providers yourself:
  - HTTP server: the Gin middleware is registered globally in each service — no per-route work.
  - Outbound HTTP: `internal/httpf` clients are already instrumented (otelhttp roundtripper + span attributes); call sites get tracing for free.
  - Database: open SQL handles via `database.OpenInstrumentedSQL` / `database.OpenInstrumentedConnector` (used by both `internal/database` and `internal/request_log`).
  - Redis: hook installation is shared in `internal/apredis`.
  - Asynq: middleware is registered by `service.RegisterTasks`; periodic tasks come through `PeriodicTaskManager`.
  - Logs ↔ traces: `internal/aplog` uses the `otelslog` bridge — emit normal `slog` records and trace/span IDs are attached automatically.
- **Labels are projected, never re-merged.** Spans and metrics read the already-resolved label set from `RequestInfo.Labels` (the namespace → connector → connection → request snapshot). Use `aptelemetry.LabelProjector` with the configured allowlists rather than reading raw label maps. Two independent allowlists govern span attributes vs. metric dimensions; metric allowlists should stay tight to control cardinality.
- **Health / readiness endpoints are excluded by default** so dashboards aren't drowned in liveness probes. If you add a new high-volume system endpoint, decide whether to exclude it explicitly.
- **Tests use in-memory exporters.** Pull spans/metrics with `tracetest` + a `ManualReader`; never dial a real collector from unit tests.

## Client configuration

The CLI tool (`cmd/cli`) looks for config at `~/.authproxy.yaml`:

```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```

## Key concepts

- **Connector** — YAML definition of how to authenticate to a 3rd-party service (auth type + setup flow + probes).
- **Connection** — runtime instance of a Connector, owned by an Actor, with encrypted credentials.
- **Actor** — user or service principal that owns connections; carries permissions.
- **Namespace** — hierarchical grouping for multi-tenancy (`root/...`).
