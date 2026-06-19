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

## AWS Secrets Manager Integration Test

This test hits real AWS Secrets Manager and is gated behind the `aws` build tag and an env flag.

Requirements:
- AWS credentials available via the standard AWS SDK chain.
- `AWS_REGION` set.
- `AUTH_PROXY_AWS_SECRETS_TEST=1` set to opt in.
- IAM permissions: `secretsmanager:CreateSecret`, `secretsmanager:PutSecretValue`, `secretsmanager:ListSecretVersionIds`, `secretsmanager:GetSecretValue`, `secretsmanager:DeleteSecret`.

Run:
```bash
cd integration_tests
AUTH_PROXY_AWS_SECRETS_TEST=1 AWS_REGION=us-east-1 \\
  go test -tags "integration,aws" -v ./encrypt/...
```

Notes:
- The test creates a short-lived secret and deletes it at the end.
- For CI, provide `AWS_REGION` as an environment variable and `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and optional `AWS_SESSION_TOKEN` as environment secrets.

## AWS KMS Integration Test

This test hits real AWS KMS and is gated behind the `aws` build tag and an env flag.

Requirements:
- AWS credentials available via the standard AWS SDK chain.
- `AWS_REGION` set.
- `AUTH_PROXY_AWS_KMS_TEST=1` set to opt in.
- `AUTH_PROXY_AWS_KMS_KEY_ID` set to an existing symmetric KMS key ID, key ARN, alias name, or alias ARN.
- IAM permissions: `kms:DescribeKey`, `kms:GenerateDataKey`, `kms:Encrypt`, `kms:Decrypt`.

Optional:
- `AUTH_PROXY_AWS_KMS_KEY_ID_V2` set to a second accessible symmetric KMS key to exercise provider metadata advancement and DEK rewrap.
- `AUTH_PROXY_AWS_KMS_ENDPOINT` set to override the KMS endpoint.

Run:
```bash
cd integration_tests
AUTH_PROXY_AWS_KMS_TEST=1 AWS_REGION=us-east-1 \\
  AUTH_PROXY_AWS_KMS_KEY_ID=alias/authproxy-test \\
  AUTH_PROXY_AWS_KMS_KEY_ID_V2=alias/authproxy-test-v2 \\
  go test -tags "integration,aws" -v ./encrypt/... -run TestAwsKMSKeySyncAndReencrypt
```

Notes:
- The test does not create or delete KMS keys because AWS KMS keys have delayed deletion windows.
- AWS KMS DEK generation uses `GenerateDataKey`; rewrap uses `Encrypt` / `Decrypt` and keeps the same `dek_` id.
- Set `AUTH_PROXY_AWS_KMS_KEY_ID_V2` to verify provider metadata advancement and DEK rewrap with a second key or alias.
- For CI, provide `AWS_REGION`, `AUTH_PROXY_AWS_KMS_TEST`, `AUTH_PROXY_AWS_KMS_KEY_ID`, and `AUTH_PROXY_AWS_KMS_KEY_ID_V2` as environment variables. Provide `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and optional `AWS_SESSION_TOKEN` as environment secrets. CI treats `AUTH_PROXY_AWS_KMS_KEY_ID_V2` as required so the rewrap path is covered.

## GCP Secret Manager Integration Test

This test hits real GCP Secret Manager and is gated behind the `gcp` build tag and an env flag.

Requirements:
- GCP credentials available via Application Default Credentials — typically a service account key file path in `GOOGLE_APPLICATION_CREDENTIALS`, or `gcloud auth application-default login` for local development.
- `GCP_PROJECT_ID` set to the project that owns the secrets.
- `AUTH_PROXY_GCP_SECRETS_TEST=1` set to opt in.
- IAM permissions on the project (role `roles/secretmanager.admin` covers all of these):
  - `secretmanager.secrets.create`
  - `secretmanager.secrets.delete`
  - `secretmanager.versions.add`
  - `secretmanager.versions.access`

Run:
```bash
cd integration_tests
AUTH_PROXY_GCP_SECRETS_TEST=1 GCP_PROJECT_ID=my-project \
  GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json \
  go test -tags "integration,gcp" -v ./encrypt/...
```

The test walks up from the current working directory and loads any `.env` files it finds (nearest wins), so you can drop your credentials wherever is convenient — the package directory, `integration_tests/`, the repo root, etc. Example `.env`:

```
AUTH_PROXY_GCP_SECRETS_TEST=1
GCP_PROJECT_ID=my-project
GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/sa.json
```

Notes:
- The test creates a short-lived secret and deletes it at the end.
- For CI, provide `GCP_PROJECT_ID` as an environment variable and `GOOGLE_APPLICATION_CREDENTIALS_JSON` (the full service account key JSON) as an environment secret. The GitHub Actions workflow writes the JSON to a temp file and points `GOOGLE_APPLICATION_CREDENTIALS` at it.
- The workflow runs only on pushes to `main` (and manual `workflow_dispatch`) to avoid exposing the GCP secrets to PRs from forks.

## GCP KMS Integration Test

This test hits real Google Cloud KMS and is gated behind the `gcp` build tag and an env flag.

Requirements:
- GCP credentials available via Application Default Credentials.
- `AUTH_PROXY_GCP_KMS_TEST=1` set to opt in.
- `AUTH_PROXY_GCP_KMS_KEY_NAME` set to an existing symmetric encryption CryptoKey resource name, or `GCP_PROJECT_ID`, `AUTH_PROXY_GCP_KMS_LOCATION`, `AUTH_PROXY_GCP_KMS_KEY_RING`, and `AUTH_PROXY_GCP_KMS_CRYPTO_KEY` set to its components.
- IAM permissions: `cloudkms.cryptoKeys.get`, `cloudkms.cryptoKeyVersions.useToEncrypt`, `cloudkms.cryptoKeyVersions.useToDecrypt`, and `cloudkms.locations.generateRandomBytes`.

Optional:
- `AUTH_PROXY_GCP_KMS_KEY_NAME_V2` set to a second accessible CryptoKey to exercise provider metadata advancement and DEK rewrap.
- `AUTH_PROXY_GCP_KMS_ENDPOINT` set to override the KMS endpoint.

Run:
```bash
cd integration_tests
AUTH_PROXY_GCP_KMS_TEST=1 \
  AUTH_PROXY_GCP_KMS_KEY_NAME=projects/my-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper \
  AUTH_PROXY_GCP_KMS_KEY_NAME_V2=projects/my-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper-v2 \
  GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json \
  go test -tags "integration,gcp" -v ./encrypt/... -run TestGcpKMSKeySyncAndReencrypt
```

Notes:
- The test does not create or delete KMS keys because Google Cloud KMS keys are long-lived resources.
- GCP KMS DEK generation uses `GenerateRandomBytes` and then wraps the generated DEK with the configured CryptoKey.
- Set `AUTH_PROXY_GCP_KMS_KEY_NAME_V2` to verify provider metadata advancement and DEK rewrap with a second CryptoKey.
- For CI, provide `GCP_PROJECT_ID`, `AUTH_PROXY_GCP_KMS_TEST`, `AUTH_PROXY_GCP_KMS_KEY_NAME`, and `AUTH_PROXY_GCP_KMS_KEY_NAME_V2` as environment variables. Provide `GOOGLE_APPLICATION_CREDENTIALS_JSON` as an environment secret. CI treats `AUTH_PROXY_GCP_KMS_KEY_NAME_V2` as required so the rewrap path is covered.

## HashiCorp Vault Integration Test

These tests hit a running HashiCorp Vault server and are gated behind the `vault` build tag and an env flag. Unlike the AWS and GCP tests, Vault runs locally (in dev mode) both on developer machines and in CI — no external credentials required.

Requirements:
- A Vault server reachable at `VAULT_ADDR` with a token in `VAULT_TOKEN` that can read and write the `secret/` KV v2 mount, enable/read/write the `transit/` mount, and rotate Transit keys.
- `AUTH_PROXY_VAULT_TEST=1` set to opt in.

The `docker-compose.yml` in this directory already includes a Vault dev-mode service with the root token `dev-only-token`. Bring it up with `docker compose up -d` and point the test at it:

```bash
cd integration_tests
AUTH_PROXY_VAULT_TEST=1 \
  VAULT_ADDR=http://127.0.0.1:8200 \
  VAULT_TOKEN=dev-only-token \
  go test -tags "integration,vault" -v ./encrypt/... -run 'TestVault(KeySyncAndReencrypt|TransitKeySyncAndReencrypt)'
```

Or start Vault ad hoc:

```bash
docker run -d --name vault -p 8200:8200 --cap-add=IPC_LOCK \
  -e VAULT_DEV_ROOT_TOKEN_ID=dev-only-token \
  hashicorp/vault:1.20.4 server -dev
```

Notes:
- The KV test writes a short-lived KV v2 secret at a unique path and deletes its metadata on cleanup.
- The Transit test creates a unique Transit key, generates a DEK through `datakey/plaintext`, rotates the Transit key, verifies DEK rewrap, and decrypts through a fresh encrypt service cache.
- CI uses `.github/workflows/vault-integration.yml`, which runs Vault as a dev-mode container inside the workflow. The workflow runs on pushes to `main` and on `workflow_dispatch`.

Focused KMS provider commands:

```bash
cd integration_tests

# AWS KMS
AUTH_PROXY_AWS_KMS_TEST=1 AWS_REGION=us-east-1 \
  AUTH_PROXY_AWS_KMS_KEY_ID=alias/authproxy-test \
  AUTH_PROXY_AWS_KMS_KEY_ID_V2=alias/authproxy-test-v2 \
  go test -tags "integration,aws" -v ./encrypt/... -run TestAwsKMSKeySyncAndReencrypt

# Google Cloud KMS
AUTH_PROXY_GCP_KMS_TEST=1 \
  AUTH_PROXY_GCP_KMS_KEY_NAME=projects/my-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper \
  AUTH_PROXY_GCP_KMS_KEY_NAME_V2=projects/my-project/locations/global/keyRings/authproxy/cryptoKeys/dek-wrapper-v2 \
  GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa.json \
  go test -tags "integration,gcp" -v ./encrypt/... -run TestGcpKMSKeySyncAndReencrypt

# Vault Transit
AUTH_PROXY_VAULT_TEST=1 \
  VAULT_ADDR=http://127.0.0.1:8200 \
  VAULT_TOKEN=dev-only-token \
  go test -tags "integration,vault" -v ./encrypt/... -run TestVaultTransitKeySyncAndReencrypt
```

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
- **OAuth test provider** ([rmorlok/go-oauth2-server](https://github.com/rmorlok/go-oauth2-server)) on port 8086, running in `--test-mode` (embedded SQLite, no remote config). The `/test/*` control plane lets tests register clients/users, drive authorize programmatically, script responses, and inspect recorded requests. See [test_mode_api.md](https://github.com/rmorlok/go-oauth2-server/blob/main/docs/test_mode_api.md) for the full API; `helpers.OAuth2TestProvider` wraps it.

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
│   ├── setup.go                 # IntegrationTestEnv creation, NewNoAuthConnector, NewOAuth2Connector
│   ├── oauth2_provider.go       # OAuth2TestProvider — wraps go-oauth2-server's /test/* control plane
│   ├── testserver.go            # In-process configurable HTTP test servers
│   ├── noop_roundtripper.go     # No-op request log middleware
│   └── util.go                  # Small utilities (JSON marshaling, etc.)
├── oauth2/                      # OAuth2 integration tests (drive go-oauth2-server in test mode)
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
// Use env.ApiGin with httptest.ResponseRecorder
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
