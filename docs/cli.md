# Command-line interface (`ap`)

The `ap` CLI under [`cmd/cli/`](../cmd/cli/) is the developer/operator tool for talking to a running AuthProxy server: listing connectors and connections, signing JWTs to drive scripted calls, and running local reverse proxies that route through a connection's credentials.

Build/run:

```bash
go run ./cmd/cli <command>           # from a checkout
go install ./cmd/cli && ap <command> # installed as $GOBIN/ap
```

The rest of this doc uses `ap <command>` for brevity.

## Configuration

### The config file

`ap` reads `~/.authproxy.yaml` by default. Override with `--config <path>`.

```yaml
# Default actor signed onto outbound requests. If omitted, --actorId on
# the command line is required (or `--admin` falls back to the current
# OS username).
admin_username: bobdole

# Asymmetric signing key (RS256). Use this for the standard dev setup —
# the public key lives in the server's system_auth.jwt_signing_key block.
admin_private_key_path: /path/to/private/key

# Optional alternative: HMAC shared secret (HS256). Mutually exclusive
# with admin_private_key_path on a given invocation.
# admin_shared_key_path: /path/to/shared.secret

# Base URLs for each AuthProxy service. Only the services you call need
# to be set — `api` is the most common for everyday CLI use.
server:
  api: http://localhost:8081
  admin_api: http://localhost:8082
  auth: http://localhost:8080
  marketplace: http://localhost:5173
  admin_ui: http://localhost:5174

# Defaults for `ap signing-proxy`. Useful when several AuthProxy clones
# share one machine — each clone's signing-proxy needs its own port. The
# `env_var` form lets one shared ~/.authproxy.yaml pick up the per-clone
# value from the clone's .env (AUTHPROXY_SIGNING_PROXY_PORT).
signing_proxy:
  port:
    env_var: AUTHPROXY_SIGNING_PROXY_PORT
    default: "8888"
```

Every value here can be overridden per-invocation by the flag of the same shape (`--apiUrl`, `--privateKeyPath`, `--actorId`, `--port`, etc.).

### Signing keys

AuthProxy supports two JWT signing modes:

| Mode | YAML field / flag | Server-side counterpart |
|---|---|---|
| RS256 (asymmetric) | `admin_private_key_path` / `--privateKeyPath` | `system_auth.jwt_signing_key.public_key.path` |
| HS256 (shared secret) | `admin_shared_key_path` / `--secretKeyPath` | `system_auth.jwt_signing_key.private_key.path` (used as the shared secret) |

The dev stack ships a ready-to-use RSA keypair under [`dev_config/keys/admin/`](../dev_config/keys/admin/) (`bobdole` + `bobdole.pub`) paired with the matching public key registered in `dev_config/default.yaml`. Point `admin_private_key_path` at `dev_config/keys/admin/bobdole` and you're signed in as `bobdole` against a fresh `docker compose up -d` server.

### Actor and scope

Every signed token has an **actor** (who is making the call) and a **service-id allowlist** (which AuthProxy services the token is valid against).

- Actor defaults: `--actorId` > YAML `admin_username` > current OS username (only when `--admin` is set).
- Service allowlist defaults to `all`. Override with `--apis admin-api,api` to scope a token down. Valid IDs: `admin-api`, `api`, `public`, `worker`.
- `--admin` flips the token's permissions to match the `system_auth.actors.permissions` block on the server (full access in the dev config).

## Commands

### `ap list connectors` / `ap list connections`

Paginates `GET /api/v1/connectors` and `GET /api/v1/connections`, printing each item to stdout. Used to discover IDs to feed to the other commands.

```bash
ap list connectors --type oauth2 --state active --output table
ap list connections --order "created_at DESC"
```

Useful flags: `--state`, `--type` (connectors only), `--order "<field> ASC|DESC"`, plus the global output flags from the `Output` helper (`--output json|jsonl|table`, `--limit`, …).

### `ap sign-jwt`

Prints a signed JWT to stdout — useful for piping into `curl -H "Authorization: Bearer $(ap sign-jwt)"` or for debugging server-side verification.

```bash
ap sign-jwt --admin                   # sign as the current OS user, all services
ap sign-jwt --actorId alice --apis api
```

Useful scoping flags:

| Flag | Purpose |
|---|---|
| `--expires-in 24h` | Adds an expiration relative to now. Durations accept Go units plus `d` for days. |
| `--no-expiry` | Leaves the token without an expiration. Mutually exclusive with `--expires-in`. |
| `--permissions-file perms.yaml` | Adds top-level JWT permission restrictions from a YAML/JSON array or `{permissions: [...]}` object. |
| `--grafana-preset aggregate` | Adds least-privilege permissions for Grafana app-metrics time-series dashboards and live dropdown variables. Defaults expiration to 90 days. |
| `--grafana-preset logs` | Includes the aggregate preset plus `request-events:list` for request-log metadata tables. Defaults expiration to 90 days. |

Grafana datasource token examples:

```bash
# Metrics dashboards only.
ap sign-jwt --actorId grafana --apis api,admin-api --grafana-preset aggregate

# Metrics plus request-event metadata tables.
ap sign-jwt --actorId grafana --apis api,admin-api --grafana-preset logs

# Provisioned datasource token with no expiry.
ap sign-jwt --actorId grafana --apis api,admin-api --grafana-preset logs --no-expiry
```

Grafana presets use top-level JWT permissions. Those permissions only restrict the token; the backing actor still needs matching normal permissions.

### `ap verify-jwt`

Reads a JWT on stdin and verifies it against the supplied public/secret key.

```bash
echo "$TOKEN" | ap verify-jwt --publicKeyPath /etc/authproxy/keys/system.pub
```

### `ap signing-proxy`

Long-running reverse proxy that signs an admin JWT onto every forwarded request, then sends it to one of the AuthProxy services. This is the tool for driving the admin UI / marketplace SPA against a local backend without baking auth into the browser.

```bash
ap signing-proxy --proxyTo=api --port 8888
ap signing-proxy --proxyTo=admin-api --enableLoginRedirect=true
```

`--proxyTo` accepts a service id (`api`, `admin-api`, `public`) or an absolute URL. `--enableLoginRedirect` adds a `/login-redirect` handler that simulates the host application's session-initiation flow — wire `host_application.initiate_session_url` (or `admin_api.ui.initiate_session_url`) at the printed URL.

`--port` defaults to `8888`. If the CLI config sets `signing_proxy.port`, that value is used instead when `--port` is not given on the command line. Multiple clones on one machine should set `AUTHPROXY_SIGNING_PROXY_PORT` per clone in their `.env` and reference it from a shared `~/.authproxy.yaml` via `signing_proxy.port.env_var`.

### `ap proxy` — connection-scoped streaming proxy

Routes inbound requests through `POST /api/v1/connections/{id}/_proxy_raw` so the connection's credentials are applied and bodies stream end-to-end (chunked uploads, SSE responses).

**Long-running mode.** Boots a listener on `--port` (default 9999). Each inbound request derives its `X-AuthProxy-Upstream-URL` from `--upstream-base` (path + query appended) or the caller may set the header directly.

```bash
# Auto-derive upstream from base.
ap proxy --connection cxn_abc --upstream-base https://api.openai.com
curl http://127.0.0.1:9999/v1/chat/completions -d @body.json

# Caller-supplied upstream — no --upstream-base.
ap proxy --connection cxn_abc
curl http://127.0.0.1:9999/ \
  -H 'X-AuthProxy-Upstream-URL: https://api.openai.com/v1/models'
```

**One-shot mode** — append a literal `curl` or `wget` plus that tool's own argv. `ap proxy` boots an ephemeral-port listener in-process, rewrites the URL's scheme+host to point at it (path/query preserved), and execs the tool. Stdout/stderr/exit-code pass through verbatim.

```bash
ap proxy --connection cxn_abc curl https://api.openai.com/v1/models
ap proxy --connection cxn_abc curl -X POST https://api.openai.com/v1/chat/completions \
  -H 'Content-Type: application/json' -d @body.json
ap proxy --connection cxn_abc wget https://api.example.com/files/big.bin -O out.bin
```

**All `ap proxy` flags must come before the `curl`/`wget` literal** — `Flags().SetInterspersed(false)` stops cobra's flag parser at the first positional, so curl's own flags (`-X`, `-d`, `--config`, …) reach the tool without colliding with ours.

Hop-by-hop headers (RFC 7230 §6.1) are stripped on both legs of the proxy hop. Response bodies are flushed after each write so SSE streams tick out in real time.

### `ap sign-marketplace-login-url`

Generates a signed marketplace login URL — the URL the host application redirects the user to so the marketplace SPA can establish a session.

```bash
ap sign-marketplace-login-url --actorId alice
```

### `ap login-redirect`

Local stand-in for a host application's session-initiation endpoint. Hosts `/login-redirect` on the configured port and returns the same payload your real host would. Useful when developing UI changes against a local stack without spinning up the host app.

```bash
ap login-redirect --port 8889
```

## Common recipes

### Drive a probe against a connector with a scratch JWT

```bash
TOKEN=$(ap sign-jwt --admin --apis api)
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8081/api/v1/connectors/openai/probes/list-models
```

### Stream a chunked upload to MinIO through a connection

```bash
ap proxy --connection cxn_minio --upstream-base http://minio:9000 &
curl -X PUT -T big.bin http://127.0.0.1:9999/bucket/big.bin
```

### Live-tail an SSE response from an LLM connector

```bash
ap proxy --connection cxn_openai curl -N \
  https://api.openai.com/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}'
```

The `-N` (`--no-buffer`) is curl's, not ours — it disables curl's own line buffering so the tokens print as they arrive.

### Run several AuthProxy clones on one machine

Each clone (`~/src/authproxy1`, `~/src/authproxy2`, …) needs its own port pool so they don't collide. The pool lives in each clone's `.env` (gitignored) — see [`.env.example`](../.env.example) for the slot table. The `AUTHPROXY_SIGNING_PROXY_PORT` slot picks the listen port for `ap signing-proxy`:

| Clone | `AUTHPROXY_SIGNING_PROXY_PORT` |
|---|---|
| authproxy1 | 8888 (default) |
| authproxy2 | 8898 |
| authproxy3 | 8908 |
| authproxy*N* | 8888 + (N−1)·10 |

A single shared `~/.authproxy.yaml` picks up whichever clone you ran `ap` from by reading the env var loaded from that clone's `.env`:

```yaml
signing_proxy:
  port:
    env_var: AUTHPROXY_SIGNING_PROXY_PORT
    default: "8888"
```

```bash
cd ~/src/authproxy3 && ap signing-proxy --proxyTo=api   # listens on 8908
cd ~/src/authproxy1 && ap signing-proxy --proxyTo=api   # listens on 8888
ap signing-proxy --proxyTo=api --port 9000              # --port always wins
```

The matching `AUTHPROXY_HOST_APP_INITIATE_SESSION_URL` in each `.env` is templated `http://127.0.0.1:${AUTHPROXY_SIGNING_PROXY_PORT}/login-redirect`, so the server's session-initiation URL automatically matches the port the local signing-proxy listens on.

## See also

- [README — Client Config](../README.md#client-config) — short-form version of the config file.
- [AGENTS.md — Running locally](../AGENTS.md#running-locally) — the surrounding dev workflow.
- [Telemetry](telemetry.md) — what shows up in traces/metrics when these commands fire requests through the server.
