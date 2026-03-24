# AuthProxy

AuthProxy is an open-source, embeddable integration platform-as-a-service (iPaaS). It manages the full connection lifecycle to third-party systems, allowing your application to call external APIs through an authenticating proxy while keeping credentials centralized, auditable, and secure.

Instead of scattering OAuth tokens and API keys across your application, AuthProxy acts as a single control plane for all outbound integrations. Your application makes requests through the proxy, and AuthProxy handles credential injection, token refresh, and request logging transparently.

Embeddable connector marketplace UI:
![marketplace-connectors.jpg](docs/images/marketplace-connect.gif)

Pre-built admin UI for managing connectors and connections:
![admin-ui.jpg](docs/images/admin-walkthrough.gif)

## Key Concepts

### Authenticating Proxy

At its core, AuthProxy is an HTTP proxy that sits between your application and external APIs. When your application needs to call a third-party service, it sends the request to AuthProxy, which:

1. Looks up the connection and its stored credentials
2. Injects the appropriate authentication (OAuth2 bearer token, API key, etc.)
3. Forwards the request to the external service
4. Returns the response to your application
5. Logs the request for auditability

If an OAuth2 access token has expired, AuthProxy automatically refreshes it using the stored refresh token before forwarding the request. Your application never needs to handle token lifecycle management.

```mermaid
sequenceDiagram
    participant App as Your Application
    participant AP as AuthProxy
    participant API as External API

    App->>AP: POST /connections/{id}/_proxy (no credentials)
    AP->>AP: Look up connection, inject auth
    AP->>API: POST /api/resource (with Bearer token)
    API-->>AP: 200 OK
    AP->>AP: Log request/response
    AP-->>App: 200 OK
```

### Connectors and Connections

A **connector** defines how to authenticate with a specific external service. It is a declarative specification that includes the authentication method (OAuth2, API key, or no auth), required scopes, token endpoints, and validation probes. Connectors are defined in YAML configuration and stored as immutable, versioned records.

A **connection** is a runtime instance of an authenticated session with an external service. When a user authorizes access through the OAuth2 flow (or provides an API key), AuthProxy creates a connection that stores the encrypted credentials. Connections are owned by actors and scoped to namespaces.

Supported authentication methods:

- **OAuth2**: Full authorization code flow with automatic token refresh and revocation
- **API Key**: Injected as a header, query parameter, or request body field
- **No Auth**: Pass-through proxy for services that don't require authentication

### Connector Versioning

Connector definitions are **immutable once published**. When you need to change a connector's configuration (e.g., adding new OAuth scopes or updating an endpoint), you create a new version rather than modifying the existing one. This ensures that live connections are never disrupted by configuration changes.

Each connector version progresses through a lifecycle:

- **Draft**: Being developed. Not available for new connections.
- **Primary**: The published version used for new connections. Existing connections on older versions are upgraded when possible.
- **Active**: A previously primary version that still has connections that haven't been upgraded.
- **Archived**: An old version with no remaining active connections.

This versioning model enables progressive rollout: you can publish a new version, let connections migrate gradually, and roll back by promoting an older version to primary if issues arise.

### Namespaces and Access Control

AuthProxy uses a **hierarchical namespace model** for organizing resources and controlling access. Rather than assigning ownership directly to resources, access is controlled through namespace-scoped permissions.

Namespaces are dot-separated paths that form a tree:

```
root
root.team-alpha
root.team-alpha.project-1
root.team-beta
```

Each team or user gets a nested namespace. Permissions granted at a parent namespace cascade to children, so an administrator with access to `root.team-alpha` automatically has access to `root.team-alpha.project-1`.

Permissions are defined as a combination of **namespace**, **resources**, and **verbs**:

```json
{
  "namespace": "root.team-alpha.**",
  "resources": ["connections"],
  "verbs": ["get", "list", "proxy"]
}
```

Wildcards (`*`) are supported for resources and verbs. The `**` suffix on namespaces matches all descendants.

### JWT Authentication and Scoped Tokens

AuthProxy uses JWTs for authentication with a flexible model designed for **least-privilege access**. Actors (users or service accounts) have a set of permissions stored in the database, but each JWT issued can carry a **subset** of those permissions.

This per-token scoping is particularly useful for **agent-based workflows**. When an AI agent or automated process acts on behalf of a user, you can issue a JWT that includes only the specific permissions the agent needs, even though the user may have broader access. This limits the blast radius if a token is compromised and enforces the principle of least privilege.

```yaml
# Full user permissions
permissions:
  - namespace: "root.team-alpha.**"
    resources: ["*"]
    verbs: ["*"]

# Scoped JWT for an agent acting on behalf of this user
permissions:
  - namespace: "root.team-alpha.project-1"
    resources: ["connections"]
    resource_ids: ["conn_abc123"]
    verbs: ["proxy"]
```

### Developer-First Authentication

AuthProxy is built with a developer-first orientation. Developers can register local SSH keys or private keys to sign JWTs directly, enabling self-signed requests to a deployed AuthProxy server without needing a separate authentication flow.

This means:

- **Local development**: Developers can interact with a deployed AuthProxy instance using their own SSH key, without needing shared secrets or a running auth server.
- **CLI tooling**: The `authproxy` CLI reads a private key path from `~/.authproxy.yaml` and signs requests automatically.
- **Agent integration**: Automated agents can use dedicated key pairs to authenticate, with permissions controlled by their actor configuration.

The system supports both **actor-signed tokens** (asymmetric, using the actor's private key) and **system-signed tokens** (symmetric HMAC, used for internal service-to-service communication).

### Application-Level Encryption

AuthProxy employs **application-level encryption** (ALE) for all sensitive data. OAuth tokens, API keys, and connector definitions are encrypted before they reach any data store — whether that's the primary database, Redis, or object storage. Direct access to the database does not provide a path to view credentials; an attacker with a database dump sees only ciphertext.

All encrypted values use AES-GCM and are stored in a self-describing format that embeds the encryption key version ID alongside the ciphertext:

```json
{"id": "ekv_abc123", "d": "base64-encoded-ciphertext"}
```

This means the system can always determine which key encrypted a given value without external metadata.

#### Namespace-Scoped Keys

Encryption keys follow the same hierarchical namespace model as the rest of AuthProxy. A global AES key (configured at startup) serves as the root, and each namespace can optionally define its own encryption key. When encrypting data for a namespace, the system resolves the key by walking up the namespace tree:

```
root.tenant-a.app1  →  root.tenant-a  →  root  →  global key
```

The first namespace with an assigned encryption key is used. Child namespaces inherit their parent's key unless they explicitly set their own. This enables **per-tenant key isolation**: a customer can bring their own encryption key so that their data is cryptographically separated from other tenants, even within a shared database.

Keys can be sourced from external secret managers including AWS Secrets Manager, GCP Secret Manager, HashiCorp Vault, environment variables, or the filesystem — allowing customers to retain control of their key material.

#### Automatic Key Rotation and Re-encryption

Key rotation is fully automatic. When a key is rotated in the external provider (e.g., a new version is created in AWS Secrets Manager), AuthProxy detects the change through a periodic sync process and begins using the new version for all new encryptions immediately.

A background re-encryption task then scans all tables with encrypted columns, compares each field's key version against the namespace's target version, and re-encrypts any mismatched fields with the current key. This happens in batches with no downtime and no application changes required.

The same mechanism handles **namespace-level key changes**. If a namespace's encryption key is reassigned — for example, migrating a tenant to their own dedicated key — all data within that namespace is automatically re-encrypted with the new key. Old key versions are retained for decryption during the transition and can be removed once re-encryption is complete.

### Embeddable Marketplace

AuthProxy includes a **marketplace UI** that can be embedded in your host application. This React-based single-page application provides a ready-made interface for users to browse available connectors, establish connections, and manage their integrations.

The marketplace uses a **delegated session model**: your host application generates a short-lived nonce JWT for the current user and redirects to the marketplace SPA. The SPA exchanges this token for a session with AuthProxy. If validation fails, the user is redirected back to your application's login flow.

```mermaid
sequenceDiagram
    participant User
    participant Host as Host Application
    participant SPA as Marketplace SPA
    participant AP as AuthProxy Public

    User->>Host: Navigate to marketplace
    Host->>Host: Generate nonce JWT for actor
    Host->>SPA: Redirect with auth token
    SPA->>AP: POST /session/_initiate
    AP-->>SPA: Session cookie + config
    SPA->>AP: GET /api/connectors
    SPA->>AP: GET /api/connections
    SPA->>SPA: User manages connections
```

This keeps AuthProxy's session lifecycle decoupled from your application's authentication system while providing a seamless user experience.

### Request Logging and Auditability

Every request proxied through AuthProxy is logged with comprehensive metadata for auditability:

- HTTP method, URL, status code, and duration
- Request and response size and content types
- Connection, connector, and connector version references
- Namespace and correlation IDs for tracing
- Custom labels for filtering

AuthProxy can optionally record **full request and response bodies**, which is configurable per-connector or globally. This is invaluable for building and debugging integrations, as you can replay exactly what was sent and received. Body recording respects configurable size limits, content type filters, and sensitive data redaction rules.

Request logs are stored in a pluggable backend (SQLite, PostgreSQL, or ClickHouse) with full bodies stored in object storage (MinIO/S3).

### Labels and Annotations

AuthProxy provides a **Kubernetes-style label system** on all major resources (namespaces, actors, connections, connectors, and request logs). Labels are key-value pairs that follow a structured format:

```
app.example.com/tenant-id: "tenant-123"
app.example.com/environment: "production"
```

Labels serve as a **lightweight integration layer** between AuthProxy and your host application. Instead of maintaining a separate mapping table to associate AuthProxy resources with your application's data model, you can attach labels directly to AuthProxy resources and query them using label selectors.

Use cases include:

- **Tenant mapping**: Tag connections with your application's tenant or user IDs
- **Environment tagging**: Distinguish production, staging, and development connections
- **Connector discovery**: Find connector versions matching specific criteria
- **Request filtering**: Search audit logs by custom dimensions
- **Workflow tracking**: Mark connections with pipeline or workflow identifiers

Labels support Kubernetes-style selector syntax for querying, making it easy to find resources matching complex criteria.

## Architecture

AuthProxy runs as multiple independent services that can be started together or separately:

| Service | Default Port | Purpose |
|---------|-------------|---------|
| **public** | 8080 | OAuth callbacks, marketplace SPA, session management |
| **api** | 8081 | Core API for application integration and proxying |
| **admin-api** | 8082 | Administrative API with UI for managing connectors and connections |
| **worker** | 8083 (health) | Background task processor (token refresh, connection upgrades) |

All services share the same database and Redis instance. The separation allows independent scaling and different security contexts (e.g., the public service handles OAuth callbacks that must be internet-accessible, while the API service can be internal).

## Running Locally

### Quick Start with Docker Compose

The fastest way to get started is with Docker Compose, which manages all dependencies for you.

**Prerequisites:** Install [Docker Desktop](https://www.docker.com/products/docker-desktop/).

**Start data stores only** (for local Go/frontend development):

```bash
docker compose up -d
```

This starts PostgreSQL (5432), Redis (6379), MinIO (9000/9001), and ClickHouse (8123/9009).

Then run the server locally:

```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

**Start the full stack** (server runs in Docker too):

```bash
docker compose --profile server up -d
```

This also builds and starts the AuthProxy server (ports 8080-8083) using `dev_config/docker.yaml`.

**Start monitoring tools:**

```bash
docker compose --profile tools up -d
```

This adds RedisInsight on port 5540. Connect to `redis://default@redis:6379`.

**Stop everything:**

```bash
docker compose --profile server --profile tools down
```

Add `-v` to also remove data volumes.

**Reset the data environment** (tear down everything and start fresh):

```bash
./scripts/teardown-docker.sh
```

This stops all Docker containers (both docker-compose and manually-started), removes all data volumes, and cleans up the network. The next `docker compose up -d` will recreate everything from scratch.

### Manual Setup

If you prefer to manage dependencies yourself:

#### Prerequisites

Install the following on MacOS:

```bash
brew install go
brew install gh
brew install volta
brew install jq
```

Install Docker Desktop separately from their website.

Setup Javascript/Typescript dependencies:

```bash
volta install node
volta install yarn
yarn install
```

#### Start Dependencies

Create a network for the asynq system to interact with redis:

```bash
docker network create authproxy
```

Start redis (requires search module):

```bash
docker run --name redis-server -p 6379:6379 --network authproxy -d redis/redis-stack-server:latest
```

Start Postgres (for local development and tests):

```bash
docker run --name postgres-server -p 5432:5432 --network authproxy -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=authproxy -d postgres:16
```

Configure Postgres in `dev_config/default.yaml`:

```yaml
database:
  provider: postgres
  auto_migrate: true
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  database: authproxy
  sslmode: disable
```

Optionally, start ClickHouse for HTTP request logging (otherwise SQLite is used by default):

```bash
docker run \
  --name clickhouse-server \
  -p 8123:8123 \
  -p 9000:9000 \
  --network authproxy \
  -e CLICKHOUSE_DB=authproxy \
  -e CLICKHOUSE_USER=clickhouse \
  -e CLICKHOUSE_PASSWORD=clickhouse \
  -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
  -d clickhouse/clickhouse-server:latest
```

To use ClickHouse, update `dev_config/default.yaml`:

```yaml
http_logging:
  enabled: true
  full_request_recording: always
  database:
    provider: clickhouse
    auto_migrate: true
    addresses:
      - localhost:9000
    database: authproxy
```

Start MinIO (required for request log storage):

```bash
docker run --name minio -p 9002:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  --network authproxy \
  -d minio/minio server /data --console-address ":9001"
```

Create the bucket (run once):

```bash
docker run --rm --network authproxy \
  -e MC_HOST_minio=http://minioadmin:minioadmin@minio:9000 \
  minio/mc mb minio/authproxy-request-logs
```

### TLS Certificates

The dev config uses self-signed TLS certificates for the `public` (port 8080) and `admin_api` (port 8082) services. Certificates are auto-generated on first server start at `dev_config/keys/tls/` (this directory is gitignored).

To avoid browser warnings, trust the generated certificate in your system keychain. On macOS:

```bash
# Start the server once to generate the certificate, then stop it (Ctrl+C)
go run ./cmd/server serve --config=./dev_config/default.yaml all

# Add the certificate to the system keychain as a trusted root
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain \
  ./dev_config/keys/tls/cert.pem
```

After trusting the certificate, restart your browser for the change to take effect. You should then be able to visit `https://localhost:8080` and `https://localhost:8082` without TLS warnings.

If you regenerate the certificate (e.g. by deleting `dev_config/keys/tls/` and restarting the server), you will need to re-run the trust command above.

For CLI/curl usage without trusting the cert, you can skip verification:

```bash
curl -k https://localhost:8080/ping
```

Start the AuthProxy Server

```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

Run the client to proxy authenticated calls to the backend:

```bash
go run ./cmd/cli raw-proxy --enableLoginRedirect=true --proxyTo=api
```

### Testing

Run tests with SQLite (default):

```bash
go test -v ./...
```

Run tests with Postgres (ensure Postgres is running via Docker Compose or manually):

```bash
AUTH_PROXY_TEST_DATABASE_PROVIDER=postgres \
POSTGRES_TEST_HOST=localhost \
POSTGRES_TEST_PORT=5432 \
POSTGRES_TEST_USER=postgres \
POSTGRES_TEST_PASSWORD=postgres \
POSTGRES_TEST_DATABASE=postgres \
POSTGRES_TEST_OPTIONS=sslmode=disable \
go test -v ./...
```

## UI

### Marketplace UI

Run the marketplace UI:

```bash
yarn workspace @authproxy/marketplace dev
```

### Admin UI
Run the admin UI:

```bash
yarn workspace @authproxy/admin dev
```

### Viewing Redis Data

If using Docker Compose, start RedisInsight with:

```bash
docker compose --profile tools up -d
```

Then open http://localhost:5540 and connect to `redis://default@redis:6379`.

Alternatively, run RedisInsight manually:

```bash
docker run -d --name redisinsight -p 5540:5540 -v redisinsight:/data --network authproxy redis/redisinsight:latest
```

Add a connection to redis. Connect to the redis server using the following URI:

```
redis://default@redis-server:6379
```

![redis-insight-add-db.jpg](docs/images/redis-insight-add-db.jpg)

### Viewing MinIO Data

MinIO includes a web-based console for browsing stored objects (e.g. full request/response logs).

Open the MinIO Console:

```bash
open http://localhost:9001
```

Log in with:
- **Username:** `minioadmin`
- **Password:** `minioadmin`

Navigate to **Object Browser** and select the `authproxy-request-logs` bucket to view stored request log entries.

### Viewing Background Tasks
To manage tasks in asynq, install the [asynq cli](https://github.com/hibiken/asynq/blob/master/tools/asynq/README.md):

```bash
go install github.com/hibiken/asynq/tools/asynq@latest
```

and run the cli:

```bash
asynq dash
````

run the web monitoring tool:

```bash
docker run --rm \
    -d \
    --name asynqmon \
    --network authproxy \
    -p 8090:8080 \
    hibiken/asynqmon \
    --redis-addr=redis-server:6379
```

open the web ui:

```bash
open http://localhost:8090
```

![asynqmon.jpg](docs/images/asynqmon.jpg)

## Client Config

The client cli looks for a config file at `~/.authproxy.yaml`:

```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```

## Related Projects

See [RELATED.md](RELATED.md) for a list of related products in the API integration space.

## License

AuthProxy is open source. See [LICENSE](LICENSE) for details.
