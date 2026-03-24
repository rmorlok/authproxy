# Integration Tests

End-to-end tests that exercise AuthProxy features against real infrastructure (Postgres, Redis, ClickHouse, MinIO) with a full authproxy server.

## Prerequisites

Start the test infrastructure:

```bash
cd integration_tests
docker compose up -d
```

Wait for all services to be healthy:

```bash
docker compose ps
```

## Running

```bash
# Run all integration tests (TF_ACC=1 enables Terraform acceptance tests)
cd integration_tests
TF_ACC=1 go test -tags integration -v ./...

# Run proxy tests only
go test -tags integration -v ./proxy/...

# Run Terraform provider tests only (TF_ACC=1 required)
TF_ACC=1 go test -tags integration -v ./terraform/...

# Run a specific test
go test -tags integration -v -run TestRateLimiting429 ./proxy/...
```

Integration tests use the `integration` build tag so they are excluded from `go test ./...`.

## Teardown

```bash
docker compose down -v
```

## Architecture

This is a separate Go module (`integration_tests/go.mod`) that depends on the main authproxy module via a `replace` directive. Tests run against real infrastructure:

- **Postgres 16** on port 5433 (avoids conflicts with local dev on 5432)
- **Redis Stack** on port 6380 (avoids conflicts with local dev on 6379)
- **ClickHouse** on port 8124 (avoids conflicts with local dev on 8123)
- **MinIO** on port 9003 (avoids conflicts with local dev on 9000/9002)

Each test gets a full authproxy server started in-process using `service.DependencyManager` and the real `GetGinServer()` functions. The server connects to the Docker services above.

### Configuration

Test configuration is in `config/integration.yaml`. It points at the Docker services and uses development keys from `dev_config/keys/`.

## Directory structure

```
integration_tests/
├── README.md
├── go.mod                       # Separate Go module
├── docker-compose.yml           # Test infrastructure
├── config/
│   └── integration.yaml         # AuthProxy config for tests
├── helpers/                     # Shared test infrastructure
│   ├── setup.go                 # IntegrationTestEnv creation and helpers
│   ├── testserver.go            # In-process configurable HTTP test servers
│   ├── noop_roundtripper.go     # No-op request log middleware
│   └── util.go                  # Small utilities (JSON marshaling, etc.)
├── proxy/                       # Proxy/rate-limiting tests
│   └── ratelimit_test.go
├── terraform/                   # Terraform provider acceptance tests
│   └── *_test.go
└── testservers/                 # Standalone test server binaries
    └── ratelimit429/
        └── main.go
```

## Writing a new test

### 1. Create a test file

All test files must start with the build tag:

```go
//go:build integration

package mypackage
```

### 2. Set up the environment

Use `helpers.Setup` to create a fully wired environment:

```go
// For admin API tests (CRUD management, Terraform provider)
env := helpers.Setup(t, helpers.SetupOptions{
    Service:         helpers.ServiceTypeAdminAPI,
    StartHTTPServer: true,
})
defer env.Cleanup()
// Use env.ServerURL and env.BearerToken for HTTP requests

// For proxy tests (in-process gin)
env := helpers.Setup(t, helpers.SetupOptions{
    Service: helpers.ServiceTypeAPI,
    Connectors: []sconfig.Connector{
        helpers.NewNoAuthConnector(connectorID, "my-test", nil),
    },
})
defer env.Cleanup()
// Use env.Gin with httptest.ResponseRecorder
```

### 3. Make requests

For in-process proxy testing:
```go
connectionID := env.CreateConnection(t, connectorID, 1)
w := env.DoProxyRequest(t, connectionID, "http://target/path", "GET")
```

For real HTTP requests (when StartHTTPServer is true):
```go
req, _ := http.NewRequest("GET", env.ServerURL+"/api/v1/namespaces", nil)
req.Header.Set("Authorization", "Bearer "+env.BearerToken)
resp, _ := http.DefaultClient.Do(req)
```

## CI

Integration tests run as a separate job in `.github/workflows/go.yml` with real service containers (Postgres, Redis, ClickHouse, MinIO).
