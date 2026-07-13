---
title: Local development
---

Use this guide when you want fast rebuilds, frontend hot reload, optional
observability, or direct control over individual services.

## Toolchain

The backend requires Go 1.24.5 or a compatible newer release. Frontend versions
are pinned in the root `package.json` through Volta.

On macOS, one setup is:

```bash
brew install go gh jq volta
volta install node
volta install yarn
yarn install
```

Install Docker Desktop separately. Linux users can use equivalent package
manager installations.

## Run backend dependencies in Docker

```bash
docker compose up -d
```

This starts Postgres, Redis, MinIO, and ClickHouse without starting the
AuthProxy image. You can select only the services needed for an experiment:

```bash
docker compose up -d postgres redis minio minio-init clickhouse
```

The checked-in `dev_config/default.yaml` already points at these ports and
credentials.

## Run AuthProxy from source

Start all four services:

```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

The final argument can instead be `public`, `api`, `admin-api`, or `worker`.
List the registered routes with:

```bash
go run ./cmd/server routes --config=./dev_config/default.yaml
```

Verify the API at `http://localhost:8081/ping`.

## Sign into the local UIs

AuthProxy expects the host application to initiate UI sessions. During local
development, the CLI signing proxy can stand in for that host.

Create `~/.authproxy.yaml`:

```yaml
admin_username: bobdole
admin_private_key_path: /absolute/path/to/authproxy/dev_config/keys/admin/bobdole
server:
  api: http://localhost:8081
  admin_api: https://localhost:8082
  auth: https://localhost:8080
  marketplace: http://localhost:5173
  admin_ui: http://localhost:5174
```

The corresponding development public key is already registered by
`dev_config/default.yaml`. In another terminal, start the host stand-in:

```bash
go run ./cmd/cli signing-proxy \
  --enableLoginRedirect=true \
  --proxyTo=admin-api
```

See the [CLI guide](/development/cli/) for signing options and per-checkout port settings.

## Run the UIs with hot reload

```bash
yarn workspace @authproxy/marketplace dev
yarn workspace @authproxy/admin dev
```

The Marketplace defaults to `http://localhost:5173`; Admin defaults to
`http://localhost:5174`. Run the commands in separate terminals. Both use the
JavaScript SDK directly from `sdks/js/src` during development.

## Trust the development certificate

The server generates `dev_config/keys/tls/cert.pem` on first startup. On macOS,
you can trust it system-wide:

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ./dev_config/keys/tls/cert.pem
```

Restart the browser afterward. If the certificate is regenerated, trust the
new file again. For a one-off command without changing the keychain:

```bash
curl -k https://localhost:8080/ping
```

## Optional tools

Start RedisInsight at `http://localhost:5540`:

```bash
docker compose --profile tools up -d
```

Start Grafana, Prometheus, Tempo, Loki, and an OpenTelemetry Collector:

```bash
docker compose --profile observability up -d
export AUTHPROXY_OTEL_ENDPOINT=http://localhost:4317
```

Restart AuthProxy after setting the endpoint. Grafana is available at
`http://localhost:3000` without a login. See [telemetry](/operations/telemetry/)
for queries and screenshots.

## Tests

Run the default SQLite-backed Go tests:

```bash
go test ./...
```

Run them against Postgres after starting the Compose dependency:

```bash
AUTH_PROXY_TEST_DATABASE_PROVIDER=postgres \
POSTGRES_TEST_HOST=localhost \
POSTGRES_TEST_PORT=5432 \
POSTGRES_TEST_USER=postgres \
POSTGRES_TEST_PASSWORD=postgres \
POSTGRES_TEST_DATABASE=postgres \
POSTGRES_TEST_OPTIONS=sslmode=disable \
go test ./...
```

The provider-backed suites live in the separate `integration_tests` Go module;
follow its package README for prerequisites. Before every commit, run:

```bash
./scripts/preflight.sh
```

Install the repository's pre-push hook if you want that check to run on every
push:

```bash
./scripts/install-hooks.sh
```

## Clean up

```bash
./scripts/teardown-docker.sh
```

This removes the Compose profiles, manually named AuthProxy development
containers, their volumes, and the development network.
