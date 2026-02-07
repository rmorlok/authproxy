# CLAUDE.md

This file provides guidance to coding agents when working with code in this repository.

## Project Overview

AuthProxy is an open-source, embeddable integration platform-as-a-service (iPaaS). It manages the connection lifecycle to 3rd party systems, allowing applications to call those systems through an authenticating proxy.

## Development Commands

### Backend (Go)

**Start Dependencies:**
```bash
# Create network
docker network create authproxy

# Start Redis (required - includes search module)
docker run --name redis-server -p 6379:6379 --network authproxy -d redis/redis-stack-server:latest
```

**Run Server:**
```bash
go run ./cmd/server serve --config=./dev_config/default.yaml all
```

Available services: `admin-api`, `api`, `public`, `worker`, or `all`

**Run Client Proxy:**
```bash
go run ./cmd/cli raw-proxy --enableLoginRedirect=true --proxyTo=api
```

**Testing:**
```bash
# Run all tests
go test ./...

# Run tests in a specific package
go test ./internal/database

# Run a single test
go test ./internal/database -run TestActorCreate

# Run tests with verbose output
go test -v ./...
```

**Other Commands:**
```bash
# Print all routes
go run ./cmd/server routes --config=./dev_config/default.yaml
```

### Frontend (TypeScript/React)

**Setup:**
```bash
npm install -g corepack
yarn set version 4.11.0
yarn
```

**Run Marketplace UI:**
```bash
yarn workspace @authproxy/marketplace dev
```

**Run Admin UI:**
```bash
yarn workspace @authproxy/admin dev
```

### Monitoring Tools

**Redis Insight (View Redis Data):**
```bash
docker run -d --name redisinsight -p 5540:5540 -v redisinsight:/data --network authproxy redis/redisinsight:latest
# Connect to: redis://default@redis-server:6379
```

**Asynqmon (Background Tasks):**
```bash
docker run --rm -d --name asynqmon --network authproxy -p 8090:8080 hibiken/asynqmon --redis-addr=redis-server:6379
# Open: http://localhost:8090
```

**Asynq CLI:**
```bash
go install github.com/hibiken/asynq/tools/asynq@latest
asynq dash
```

## Architecture

### Service Architecture

AuthProxy runs as multiple independent services that can be started together or separately:

- **admin-api** (port 8082): Administrative API with UI for managing connectors and connections
- **api** (port 8081): Core API for application integration
- **public** (port 8080): Public-facing endpoints for OAuth callbacks and marketplace
- **worker** (port 8083 health check): Background task processor using Asynq

All services are coordinated through the `cmd/server` entrypoint which uses a service-based architecture defined in `internal/service/`.

### Core Business Logic (`internal/core`)

The core package is the central business logic layer. It wraps the database and Redis to provide fully hydrated models with methods for system interaction. Core handles:
- Connectors (3rd party service definitions)
- Connections (authenticated instances to external services)
- Actors (users/entities that own connections)
- Automatic logging, event queuing, and background task scheduling

**Important:** Other packages should depend on `internal/core/iface` (interfaces) rather than `internal/core` directly to maintain proper layering.

### Database Layer (`internal/database`)

The database package provides direct SQL access using:
- SQLite (default, configured via YAML)
- Database migrations in `internal/database/migrations/sqlite/`
- Squirrel for query building
- Models: Actor, Connection, Connector, ConnectorVersion, Namespace, OAuth2Token

### Authentication & Sessions (`internal/auth`)

Handles session management across services:
- Reads and validates JWTs from requests
- Verifies tokens against database state
- Injects session information into request context
- Uses public/private key pairs for JWT signing (configured in `system_auth.jwt_signing_key`)

### JWT Package (`internal/jwt`)

Manages JWT operations:
- Actor-based token signing and verification
- System-level authentication tokens
- Integrates with the key configuration system

### Background Tasks (`internal/tasks`, `internal/apasynq`)

- `internal/tasks`: API-exposed task wrappers bound to specific auth contexts for secure customer monitoring
- `internal/apasynq`: Extracted interface for Asynq with centralized mocking capabilities
- Uses Redis and Asynq for task queuing

### Configuration (`internal/config`)

YAML-based configuration system with:
- Service configs (ports, TLS, CORS)
- System authentication (keys, admin users)
- Connector definitions (OAuth2, API Key, NoAuth)
- Database and Redis settings
- Logging configuration

Configuration files are in `dev_config/` for development.

### Routing (`internal/routes`)

Shared routes across services. Individual services have their own service-specific routes in `internal/service/<service>/routes`. This allows the same functionality with different security contexts (session vs. no session).

### Request Logging (`internal/request_log`)

Comprehensive HTTP request/response logging with:
- Configurable body size limits
- Content type filtering
- Sensitive data redaction
- Duration tracking in milliseconds

### Context Package (`internal/apctx`)

Provides enhanced context management:
- Correlation IDs for request tracing
- Clock interface for testable time operations
- UUID generation
- Value applier pattern for context enrichment
- Builder pattern for context construction

### Encryption (`internal/encrypt`)

Handles secure encryption operations for storing sensitive data like OAuth tokens.

### Utilities

- `internal/util`: Common utilities (JSON, regex, coercion, must, pointers, pagination)
- `internal/sqlh`: SQL helper functions for counting and scanning
- `internal/test_utils`: Testing utilities (paths, SQL helpers, logging, test data)
- `internal/httpf`: HTTP helper functions with mock support
- `internal/apredis`: Redis interface with mutex support and mocking

## Client Configuration

The CLI tool (`cmd/cli`) looks for config at `~/.authproxy.yaml`:
```yaml
admin_username: bobdole
admin_private_key_path: /path/to/private/key
server:
  api: http://localhost:8081
```

## Key Concepts

### Connectors
Connector definitions in YAML specify how to authenticate with external services. Supported auth types:
- OAuth2 (authorization code, token exchange, refresh, revocation)
- API Key (header, query param, or body)
- No Auth

Connectors can include "probes" to validate connections.

### Connections
Runtime instances of authenticated connections to external services, owned by Actors. Connections store encrypted credentials and OAuth tokens in the database.

### Actors
Actors represent users or entities that own connections. They have email, external_id, admin/super_admin flags, and permissions stored as JSON.

### Namespaces
Logical grouping for connectors and connections, allowing multi-tenancy.

## Testing Notes

- Use `internal/test_utils` for common test helpers
- Database tests use in-memory SQLite via `test_db.go`
- Mock implementations available for: Redis, Asynq, HTTP client, logging
- Tests use standard Go testing with testify for assertions
