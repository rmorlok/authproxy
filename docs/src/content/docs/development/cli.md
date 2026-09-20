---
title: Command-line interface (ap)
---

The `ap` CLI under [`cmd/cli/`](https://github.com/rmorlok/authproxy/tree/main/cmd/cli/) is the developer/operator tool for talking to a running AuthProxy server: listing connectors and connections, signing JWTs to drive scripted calls, and running local reverse proxies that route through a connection's credentials.

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
adminUsername: bobdole

# Asymmetric signing key (RS256). Use this for the standard dev setup —
# the public key lives in the server's systemAuth.jwtSigningKey block.
adminPrivateKeyPath: /path/to/private/key

# Optional alternative: HMAC shared secret (HS256). Mutually exclusive
# with adminPrivateKeyPath on a given invocation.
# adminSharedKeyPath: /path/to/shared.secret

# Base URLs for each AuthProxy service. Only the services you call need
# to be set — `api` is the most common for everyday CLI use.
server:
  api: http://localhost:8081
  adminApi: http://localhost:8082
  auth: http://localhost:8080
  marketplace: http://localhost:5173
  adminUi: http://localhost:5174

# Defaults for `ap signing-proxy`. Useful when several AuthProxy clones
# share one machine — each clone's signing-proxy needs its own port. The
# `envVar` form lets one shared ~/.authproxy.yaml pick up the per-clone
# value from the clone's .env (AUTHPROXY_SIGNING_PROXY_PORT).
signingProxy:
  port:
    envVar: AUTHPROXY_SIGNING_PROXY_PORT
    default: "8888"
```

Every value here can be overridden per-invocation by the flag of the same shape (`--apiUrl`, `--privateKeyPath`, `--actorId`, `--port`, etc.).

### Signing keys

AuthProxy supports two JWT signing modes:

| Mode | YAML field / flag | Server-side counterpart |
|---|---|---|
| RS256 (asymmetric) | `adminPrivateKeyPath` / `--privateKeyPath` | `systemAuth.jwtSigningKey.publicKey.path` |
| HS256 (shared secret) | `adminSharedKeyPath` / `--secretKeyPath` | `systemAuth.jwtSigningKey.privateKey.path` (used as the shared secret) |

The dev stack ships a ready-to-use RSA keypair under [`dev_config/keys/admin/`](https://github.com/rmorlok/authproxy/tree/main/dev_config/keys/admin/) (`bobdole` + `bobdole.pub`) paired with the matching public key registered in `dev_config/default.yaml`. Point `adminPrivateKeyPath` at `dev_config/keys/admin/bobdole` and you're signed in as `bobdole` against a fresh `docker compose up -d` server.

### Actor and scope

Every signed token has an **actor** (who is making the call) and a **service-id allowlist** (which AuthProxy services the token is valid against).

- Actor defaults: `--actorId` > YAML `adminUsername` > current OS username (only when `--admin` is set).
- Service allowlist defaults to `all`. Override with `--apis admin-api,api` to scope a token down. Valid IDs: `admin-api`, `api`, `public`, `worker`.
- `--admin` flips the token's permissions to match the `systemAuth.actors.permissions` block on the server (full access in the dev config).

The CLI emits a subject-only token: the selected actor ID is placed in `sub`,
and the server resolves the existing actor in the selected namespace. It does
not embed or provision an actor resource. Applications that do embed actors
must use the restricted `authproxy.net/v1alpha1` Actor claim described in
[Authentication and Authorization](/security/authentication-and-authorization/#authentication-paths).

## Commands

### `ap list connectors` / `ap list connections`

Paginates `GET /api/v1/connectors` and `GET /api/v1/connections`, printing each
item to stdout. Output includes the human-readable name and immutable ID. Use
the name to recognize a resource, but pass the ID to commands and URLs that
address it directly.

```bash
ap list connectors --name salesforce --state active
ap list connections --name production-crm --order "created_at DESC"
```

`--name` is an exact, case-sensitive filter. If the caller can list several
namespaces, the same name can produce more than one result; compare namespace
and ID before acting. Other useful flags are `--state`, `--type` (connectors
only), and `--order "<field> ASC|DESC"`. Pagination is automatic and the
complete result is printed as a JSON array.

JSON output contains complete `authproxy.net/v1alpha1` resources, including
`apiVersion`, `kind`, `metadata`, `spec`, and (when available) `status`.
Scripts that need durable identity should read `metadata.id`; the display name
is `metadata.name`, and connector generations use `metadata.generation`. The
CLI does not yet have dedicated create or rename commands, so use the API for
those operations.
See [Resource identity and names](/reference/api/#resource-identity-and-names)
for create, rename-by-ID, conflict, and query examples.

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

Grafana presets use top-level JWT permissions. Those permissions only restrict
the token; the backing actor still needs matching normal permissions. The
server rejects a token whose top-level permissions are broader than the
backing actor's grants.

Permission namespaces in a permissions file support the same actor templates as normal actor permissions: `{{externalId}}`, `{{labels.<label>}}`, and `{{annotations.<annotation>}}`. These render against the backing actor, and missing label or annotation values make the permission fail to match.

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

`--proxyTo` accepts a service id (`api`, `admin-api`, `public`) or an absolute URL. `--enableLoginRedirect` adds a `/login-redirect` handler that simulates the host application's session-initiation flow — wire `hostApplication.initiateSessionUrl` (or `adminApi.ui.initiateSessionUrl`) at the printed URL.

`--port` defaults to `8888`. If the CLI config sets `signingProxy.port`, that value is used instead when `--port` is not given on the command line. Multiple clones on one machine should set `AUTHPROXY_SIGNING_PROXY_PORT` per clone in their `.env` and reference it from a shared `~/.authproxy.yaml` via `signingProxy.port.envVar`.

### `ap proxy` — connection-scoped streaming proxy

Routes inbound requests through `POST /api/v1/connections/{id}/_proxyRaw` so the connection's credentials are applied and bodies stream end-to-end (chunked uploads, SSE responses).

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

Each clone (`~/src/authproxy1`, `~/src/authproxy2`, …) needs its own port pool so they don't collide. The pool lives in each clone's `.env` (gitignored) — see [`.env.example`](https://github.com/rmorlok/authproxy/blob/main/.env.example) for the slot table. The `AUTHPROXY_SIGNING_PROXY_PORT` slot picks the listen port for `ap signing-proxy`:

| Clone | `AUTHPROXY_SIGNING_PROXY_PORT` |
|---|---|
| authproxy1 | 8888 (default) |
| authproxy2 | 8898 |
| authproxy3 | 8908 |
| authproxy*N* | 8888 + (N−1)·10 |

A single shared `~/.authproxy.yaml` picks up whichever clone you ran `ap` from by reading the env var loaded from that clone's `.env`:

```yaml
signingProxy:
  port:
    envVar: AUTHPROXY_SIGNING_PROXY_PORT
    default: "8888"
```

```bash
cd ~/src/authproxy3 && ap signing-proxy --proxyTo=api   # listens on 8908
cd ~/src/authproxy1 && ap signing-proxy --proxyTo=api   # listens on 8888
ap signing-proxy --proxyTo=api --port 9000              # --port always wins
```

The matching `AUTHPROXY_HOST_APP_INITIATE_SESSION_URL` in each `.env` is templated `http://127.0.0.1:${AUTHPROXY_SIGNING_PROXY_PORT}/login-redirect`, so the server's session-initiation URL automatically matches the port the local signing-proxy listens on.

## See also

- [Local development](/development/local-development/) — the surrounding source and UI workflow.
- [AGENTS.md — Running locally](https://github.com/rmorlok/authproxy/blob/main/AGENTS.md#running-locally) — repository-specific contributor guidance.
- [Telemetry](/operations/telemetry/) — what shows up in traces/metrics when these commands fire requests through the server.

## Apply resource manifests

`ap apply` creates or updates resources through the configured API service. It
validates the complete input and its prerequisites before writing, then executes
in dependency order. Use `--dry-run=client` to check resource types, fields,
identity, and namespace selection without contacting the cluster.

```bash
ap apply -f ./resources --recursive --namespace root.integrations
ap apply -f ./resources --recursive --namespace root.integrations --dry-run=client
ap apply -f namespaces.yaml -f connectors.yaml --dry-run=client -o yaml
cat resources.yaml | ap apply -f - --namespace root.integrations --dry-run=client
```

For example, save this as `namespace.yaml`:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata:
  name: integrations
  namespace: root
spec: {}
```

Then run `ap apply -f namespace.yaml --dry-run=client`. This validates the
namespace `root.integrations`; it does not create it.

### Identity and input rules

Resources must contain `metadata.id` or both `metadata.namespace` and
`metadata.name`. An explicit namespace takes precedence over `--namespace`.
If a name-based resource omits its namespace and no flag supplies it, validation
fails. There is no implicit `root` default. ID-only targets need no namespace;
cluster execution resolves them and checks any supplied identity fields.
An explicit missing ID fails instead of creating a different resource.

For `Namespace`, `metadata.namespace` is the parent path. A namespace ID supplies
its canonical path, and any supplied name or parent must agree. `name: root`
without a parent identifies the root namespace, which does not receive the
flag's default parent.

Supported kinds are `Namespace`, `Actor`, `Connector`, `Key`, `RateLimit`, and
`Connection`, using `apiVersion: authproxy.net/v1alpha1`. Connection input is
limited to mutable metadata and an omitted or empty spec; connection setup is
not declarative creation. Status and timestamps are rejected. Only Connector
manifests may address a generation.

The loader accepts YAML, JSON, multi-document YAML, typed resource lists such as
`ActorList`, and heterogeneous `List` envelopes. List items must carry their own
API version and kind. Incomplete paginated lists are rejected. Duplicate keys,
YAML aliases/merge keys, unknown fields in strict mode, invalid identity, and duplicate selected
resource identities fail the whole batch. Diagnostics identify the source,
document, and list item. Multiple inputs retain their argument order; directory
files are visited lexically. Symlinks discovered inside directories are skipped.

### Supported operations

| Kind | Create | Update |
|---|---|---|
| `Namespace` | Name and parent path | Mutable metadata and namespace settings; the path is immutable. |
| `Actor` | Name, namespace, and actor spec | Mutable metadata and actor settings, including explicitly supplied signing credentials. |
| `Connector` | Name, namespace, and definition | Mutable metadata, draft definitions, and publication intent; see generations below. |
| `Key` | Name, namespace, and key settings | Mutable metadata and supported key settings. Available through both API services; `--admin` selects the admin API. |
| `RateLimit` | Name, namespace, and rate-limit policy | Mutable metadata and policy. |
| `Connection` | Unsupported | Existing connection's mutable metadata only. Establish connections through the connection setup flow. |

Canonical resource validation still applies: required fields cannot be removed,
and immutable fields cannot be changed. Object fields are merged by child key;
use explicit `null` to clear a nullable reference such as
`spec.encryptionKeyRef`, including any ID the server added when resolving it.
Removing a document from your input
does not delete its resource.

### Create, edit, and adopt resources

This multi-document file creates a namespace and an actor inside it. Apply orders
the namespace first even if the actor appears first in the file:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata:
  name: integrations
  namespace: root
spec: {}
---
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata:
  name: worker
  namespace: root.integrations
  labels:
    environment: development
    temporary: "true"
spec:
  externalId: integration-worker
```

Run `ap apply -f resources.yaml`, then edit `environment` and remove `temporary`
and apply again. Apply updates the managed label and removes the omitted label;
labels added independently are preserved. Server-projected `apxy/` labels are
read-only and are never submitted in patches. A later apply with the same desired
state reports `unchanged`. An initial reapply may update history to include the
server-assigned ID before reaching that steady state.

To adopt an existing resource, use its namespace and name, or supply its ID:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata:
  id: act_REPLACE_WITH_EXISTING_ID
  labels:
    environment: production
spec: {}
```

Replace the placeholder with an actual actor ID. With no previous apply history,
apply warns and takes ownership of the supplied fields; it preserves omitted
fields. Do not copy a GET response directly into a manifest: remove status,
timestamps, and redacted credentials first. Structured apply output also contains
result envelopes rather than replayable manifests.

### Available options

| Option | Behavior |
|---|---|
| `-f`, `--filename` | Repeatable file, directory, HTTP(S) URL, or `-` for stdin. Stdin may appear only once. |
| `-R`, `--recursive` | Traverse nested directories. Directory discovery includes `.yaml`, `.yml`, and `.json`. |
| `-n`, `--namespace` | Supply a missing namespace or Namespace parent. |
| `-l`, `--selector` | Filter input labels using `=`, `==`, `!=`, existence (`key`), or nonexistence (`!key`). |
| `--dry-run` | `none` (default) applies to the cluster; `client` validates without cluster access or signing configuration. |
| `--validate` | `strict` (default) rejects unknown fields; `warn` drops them with warnings; `ignore` drops them silently. `true` aliases strict and `false` aliases ignore. |
| `--overwrite` | Defaults to `true`; controls managed-field drift during reconciliation. Has no effect during client dry-run. |
| `-o`, `--output` | `name`, `json` (an array), or `yaml` (a document stream). Execution prints operation results; client dry-run prints desired resources. The default prints human-readable status. |
| `--request-timeout` | Timeout per cluster request, default `30s`; `0` disables it. |

The loader checks every resource before producing output, including resources
excluded by a selector. Validation modes never permit malformed identities,
duplicate keys, invalid field types, server-owned fields, or redacted secret
placeholders. Warn/ignore remove unknown fields before output. A selector matching no resources succeeds with empty
output (`[]` for JSON); input containing no resource documents fails.

URL downloads use an independent, unsigned HTTP client, a 30-second timeout,
and a 16 MiB limit per source. URL credentials and HTTPS-to-HTTP redirects are
rejected. Local files and stdin use the same size limit. An HTTP source still
requires network access during client dry-run.

Structured output masks schema-declared secrets and retains explicit null and
empty values. Redacted output is for inspection, not a replayable manifest;
redacted secret placeholders are rejected as input. Dry-run does not verify
resource existence, authorization, references, server defaults, or whether a
subsequent apply would create or update a resource.

### Execution and failures

Cluster execution uses the existing signing and configuration flags, including
`--config`, `--actorId`, `--privateKeyPath`/`--secretKeyPath`, `--apiUrl`, and
`--admin` with `--adminApiUrl`. It requires read access to namespace and explicit
reference prerequisites as well as the resource read/create/patch permissions.
Logical Connector applies also require `connectors:list/generations` to inspect
the selected generation, existing draft, and newest generation.
A selector that matches no resources succeeds without connecting to the cluster.

All targets and plans are validated before writes. Newly created namespaces
precede their children and contained resources; explicit references precede their
dependents. Prerequisites must be in the selected batch or already exist. Missing
prerequisites and cycles fail before writes. For example, creating a namespace
that references an encryption key being created inside that namespace is a cycle.
When the namespace already exists, the key can be created before updating its
namespace's encryption-key reference.

History stored in `authproxy.net/last-applied-configuration` lets apply preserve
unmanaged fields and remove formerly managed omissions where supported. Secrets
are excluded from history. Explicit secret values are submitted without comparison;
omitted secrets are preserved. Because secrets cannot be compared with their
stored values, keeping an explicit secret in a manifest may cause each apply to
report `configured` rather than `unchanged`. History is stored as a resource
annotation: non-secret desired values are visible to readers of that resource.
Do not put credentials in labels, annotations, or other ordinary fields.
`--overwrite=false` reports managed-field drift
instead of overwriting it. Initial adoption of resources without history emits a
warning.

Execution results use `created`, `configured`, `unchanged`, `failed`, and `skipped`
statuses. Failed prerequisites cause dependents to be skipped; independent
resources continue. Any failure or skip returns a nonzero exit status. Cancellation
stops further mutations. JSON output is an array of envelopes containing source,
kind, identity, operation, status, a redacted resource on success, and an error on
failure. YAML emits one envelope per document. Name output prints successful
identifiers, with failures reported on stderr.

Batches are not transactional and successful writes are not rolled back. A
mutation that fails to return a valid response may already have succeeded; apply
does not retry it automatically. Results are printed after execution, so an
output error also cannot roll back writes.

Immediately before each operation, apply reads the target again and recomputes
its plan using the same overwrite policy. This preserves unrelated changes
visible at that read, including changes to labels, annotations, and apply history.
Existing resources remain bound to their resolved IDs. History removed after
preparation requires a new batch so adoption is not silent. A resource that appears
after a planned create causes a failure rather than automatic adoption; a create
conflict is also reported without retry. Review the cluster and rerun apply to
prepare a new batch. Mutation errors, including HTTP 409/412 responses, are never
replayed because the current API may have performed part of an update already.

These reads are **not atomic write preconditions**. The API does not enforce
ETags or object revisions for apply, so edits between the final read and write
can still be overwritten, including with `--overwrite=false`. Connector metadata,
history, and generation changes can also partially succeed within one request.
Serialize applies and other writers when this matters; this command does not
provide strong concurrency guarantees. Server dry-run, server-side apply, field
managers and deletion/prune flags remain unsupported.

### Connector generations

Apply inspects the selected generation and available generations before planning
logical Connector updates. It reuses an existing draft; if a draft must be
created, it merges against the newest generation that the server will clone.
Repeated applies omit known-equal definitions and do not manufacture generations.
Explicit secret values cannot be compared and may still cause definition writes.

Set `spec.release.desiredState: primary` to publish a changed definition. With no
release intent, a changed published definition becomes a draft. If a manifest
previously managed `desiredState: primary`, change it explicitly to `draft` to
stop publishing edits; omitting a required managed field cannot clear it. A declaration
already satisfied by the primary leaves an unrelated draft alone. Metadata-only
updates do not create generations.

`metadata.generation` addresses exactly that existing generation. Updates require
a draft; published generations can only return unchanged when no write (including
history adoption) is needed. Apply never calls the force-state endpoint.
