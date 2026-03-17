# Integration Tests

End-to-end tests that exercise AuthProxy features against a fully wired service stack (database, Redis, HTTP routing, encryption, auth) — all running in-process with no external dependencies.

## Running

```bash
# Run all integration tests
go test -tags integration -v ./integration_tests/...

# Run a specific test
go test -tags integration -v -run TestRateLimiting429 ./integration_tests/...
```

Integration tests use the `integration` build tag so they are excluded from `go test ./...`.

## Architecture

Tests run entirely in-process using:

- **SQLite (in-memory)** via `database.MustApplyBlankTestDbConfig`
- **miniredis** (in-process Redis) via `apredis.MustApplyTestConfigWithServer`
- **Mock asynq** for background task enqueuing
- **Real gin router** with production route handlers

No Docker, no network services, no ports to manage. Each test gets a fresh, isolated environment.

### Time control

miniredis does not expire keys based on wall-clock time. Use `env.RedisServer.FastForward(duration)` to advance Redis time and trigger TTL expiry, rather than `time.Sleep`.

## Directory structure

```
integration_tests/
├── README.md
├── helpers/                  # Shared test infrastructure
│   ├── setup.go              # IntegrationTestEnv creation and helpers
│   ├── testserver.go         # In-process configurable HTTP test servers
│   ├── noop_roundtripper.go  # No-op request log middleware
│   └── util.go               # Small utilities (JSON marshaling, etc.)
├── testservers/              # Standalone test server binaries
│   └── ratelimit429/         # Configurable 429-returning server
│       └── main.go
└── *_test.go                 # Test files (must have //go:build integration)
```

## Writing a new test

### 1. Create a test file

All test files must start with the build tag:

```go
//go:build integration

package integration_tests
```

### 2. Set up the environment

Use `helpers.Setup` to create a fully wired environment. Pass connector definitions in `SetupOptions`:

```go
env := helpers.Setup(t, helpers.SetupOptions{
    Connectors: []sconfig.Connector{
        helpers.NewNoAuthConnector(connectorID, "my-test", nil),
    },
})
defer env.Cleanup()
```

`Setup` handles: config, database migration, Redis, auth, encryption, HTTP client factory (with rate limiting middleware), core service, connector migration to DB, and gin route registration.

### 3. Create connections and make requests

```go
// Create a connection in the database
connectionID := env.CreateConnection(t, connectorID, 1)

// Make a proxy request through the full stack
w := env.DoProxyRequest(t, connectionID, "http://target/path", "GET")
```

### 4. Parse proxy responses

The proxy endpoint returns HTTP 200 with the upstream status code inside the JSON body:

```go
type proxyResponse struct {
    StatusCode int               `json:"status_code"`
    Headers    map[string]string `json:"headers"`
    BodyRaw    []byte            `json:"body_raw"`
    BodyJson   interface{}       `json:"body_json"`
}
```

### 5. Test servers

For tests that need a real upstream HTTP server, create an in-process test server:

```go
ts := helpers.NewRateLimitTestServer(t)  // auto-cleaned up via t.Cleanup
ts.SetReturn429("5")                      // toggle 429 with Retry-After header
ts.SetReturn200()                         // toggle back to 200
ts.GetRequestCount()                      // verify which requests reached upstream
```

The `testservers/` directory contains standalone binaries of these servers for use outside tests or as reference implementations.

## CI

Integration tests run as a separate job in `.github/workflows/go.yml` under `integration-test`. They require no services or Docker containers since everything runs in-process.
