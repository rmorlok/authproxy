# internal/database — Agent Guide

## Overview

This package provides the database layer for AuthProxy. It supports two providers: **SQLite** and **PostgreSQL**. Both must always be kept in sync.

## Schema & Migrations

Migration files live in two parallel directories:
- `migrations/sqlite/` — SQLite DDL (uses `datetime`, `integer` for booleans)
- `migrations/postgres/` — PostgreSQL DDL (uses `timestamptz`, `boolean`)

**Critical rule:** Every schema change must be applied to **both** migration directories. The files must define the same logical schema with provider-appropriate types. Common type mappings:

| SQLite | PostgreSQL |
|---|---|
| `datetime` | `timestamptz` |
| `integer` (0/1) | `boolean` |
| `text` | `text` |
| `text` (JSON data) | `jsonb` |

Migrations are embedded via `//go:embed migrations/**/*.sql` in `migrate.go` and applied automatically at startup using `golang-migrate`.

## Entity Pattern

Each database entity follows a consistent struct pattern:

1. **Table constant:** `const ActorTable = "actors"`
2. **Struct** with fields mapping to columns
3. **`cols()`** — returns column name slice in fixed order
4. **`fields()`** — returns pointer slice for `Scan()` in same order as `cols()`
5. **`values()`** — returns value slice for `Insert` in same order as `cols()`

When adding a column, you must update **all three methods** (`cols`, `fields`, `values`) in the same order. The order must match across all three and must match the column order in the migration.

## Soft Deletes

All entities use soft deletes via a nullable `deleted_at` column. Queries must include `WHERE deleted_at IS NULL` to exclude deleted records (use `sq.Eq{"deleted_at": nil}`).

## Encrypted Fields

Tables with encrypted data use `encfield.EncryptedField` which serializes as JSON (`{"id":"ekv_xxx","d":"base64..."}`). Fields can be pointer (`*encfield.EncryptedField`) for nullable columns or value type for required columns.

The re-encryption registry (`reencrypt_registry.go`) tracks which tables/columns contain encrypted fields. When adding a new encrypted column:
1. Add the column to both migration files
2. Add `EncryptedAt *time.Time` if the table doesn't already have it
3. Register the field in an `init()` function using `RegisterEncryptedField()`
4. Update `cols()`, `fields()`, `values()` for both the encrypted column and `encrypted_at`

## Provider-Specific SQL

SQLite and PostgreSQL have different JSON functions. When writing raw SQL expressions involving JSON:
- **SQLite:** `json_extract(col, '$.key')`
- **PostgreSQL:** `col ->> 'key'` (columns storing JSON use native `jsonb` type, no cast needed)

Use `s.cfg.GetProvider()` to branch:
```go
if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
    // Postgres syntax
} else {
    // SQLite syntax
}
```

The label selector system (`label_selector.go`) demonstrates this pattern.

## Squirrel Query Builder

All queries use the `squirrel` query builder via `s.sq`, which is pre-configured with the correct placeholder format per provider (`?` for SQLite, `$1` for Postgres). Always use `s.sq` rather than constructing queries manually.

## Interface & Mocks

The `DB` interface in `interface.go` defines all public methods. Mocks are generated with:
```bash
go generate ./internal/database/...
```

This runs `mockgen` as specified by the `//go:generate` directive in `interface.go`. After modifying the interface, always regenerate mocks.

## Testing

### Running Tests

Tests default to SQLite. To run against PostgreSQL, source the `.env` file in the project root before running tests:

```bash
# SQLite (default)
go test ./internal/database/...

# PostgreSQL — load credentials from .env, then run
set -a && source .env && set +a && go test ./internal/database/...
```

The `.env` file at the project root contains the PostgreSQL connection credentials. The test helper (`test_db.go`) calls `util.LoadDotEnv()`, which walks up from the current working directory loading every `.env` file it finds (nearest wins), so tests pick up the `.env` file when it exists. The key variables to look for in `.env` are:

- `AUTH_PROXY_TEST_DATABASE_PROVIDER` — must be set to `postgres` to enable PostgreSQL tests
- `POSTGRES_TEST_HOST` — database host (typically `localhost`)
- `POSTGRES_TEST_PORT` — database port (typically `5432`)
- `POSTGRES_TEST_USER` — database user
- `POSTGRES_TEST_PASSWORD` — database password
- `POSTGRES_TEST_DATABASE` — database name
- `POSTGRES_TEST_OPTIONS` — connection options (typically `sslmode=disable`)
- `POSTGRES_TEST_MAX_PARALLEL` — max parallel test databases
- `POSTGRES_TEST_MAX_CONNS` — max connections per test database

If the `.env` file does not exist, PostgreSQL tests cannot run. Tests will default to SQLite only, which is insufficient — schema or query differences between providers will go undetected. If you find that `.env` is missing, warn the user that PostgreSQL tests are being skipped and that both providers must be tested before changes are considered complete.

**Critical rule:** Always run tests on **both** SQLite and PostgreSQL before considering work complete.

### App metrics database (separate)

The `internal/app_metrics` package uses a *separate* database from this one and supports **three** providers: SQLite, PostgreSQL, and ClickHouse. It has its own selector env var so the two layers can be chosen independently:

- `AUTH_PROXY_APP_METRICS_TEST_DATABASE_PROVIDER` — `sqlite` | `postgres` | `clickhouse` (default `sqlite`).
- Postgres reuses the `POSTGRES_TEST_*` pool above; pgtestdb creates an isolated per-test database against that shared server.
- ClickHouse uses `CLICKHOUSE_TEST_{HOST,PORT,USER,PASSWORD,DATABASE,MAX_PARALLEL}`; the harness creates a unique per-test database (dropped on cleanup).

Use `app_metrics.MustNewBlankRequestEventsStore(t)` to get a migrated store/retriever pair driven by whichever provider is selected.

**Critical rule:** Schema changes to the app_metrics request-events table must land in **all three** migration trees (`internal/app_metrics/migrations/{sqlite,postgres,clickhouse}`) — they share a single `app_metrics_request_events` schema and the test matrix in CI will fail any cell whose provider hasn't been updated.

### Test Setup

Use `MustApplyBlankTestDbConfigRaw(t, nil)` to get a fresh migrated database with a `root` namespace pre-created. This returns `(config, DB, *sql.DB)` where the raw `*sql.DB` can be used for direct SQL when needed.

### Raw SQL in Tests

When using `rawDb.Exec()` for test setup, **do not use `?` placeholders** — they work in SQLite but fail in PostgreSQL (which expects `$1`). Instead, use `fmt.Sprintf` to inline values:

```go
// WRONG — fails on PostgreSQL
rawDb.Exec(`UPDATE namespaces SET encryption_key_id = ? WHERE path = 'root'`, ekId)

// CORRECT — works on both
rawDb.Exec(fmt.Sprintf(`UPDATE namespaces SET encryption_key_id = '%s' WHERE path = 'root'`, ekId))
```

### Timezone Handling in Tests

PostgreSQL `timestamptz` columns return times in the local timezone, while SQLite returns UTC. When comparing times in tests, use `time.Equal()` instead of `require.Equal()`:

```go
// WRONG — fails on PostgreSQL due to timezone difference
require.Equal(t, expectedTime, actualTime)

// CORRECT — compares the instant, ignoring timezone representation
require.True(t, expectedTime.Equal(actualTime))
```

### Enumerate Pattern

Pagination in enumerate methods uses a consistent pattern:
- Page size of 100
- Request `pageSize + 1` rows to detect if more pages exist
- Callback receives the page and a `lastPage` boolean
- Callback returns `(stop bool, err error)`

## Verification Checklist

After making changes to this package:

1. Both migration files updated (if schema changed)
2. Entity struct + `cols()`/`fields()`/`values()` updated in sync
3. `interface.go` updated (if public methods changed)
4. Mocks regenerated: `go generate ./internal/database/...`
5. Package builds: `go build ./internal/database/...`
6. Tests pass on SQLite: `go test ./internal/database/...`
7. Tests pass on PostgreSQL: `set -a && source .env && set +a && go test ./internal/database/...`
