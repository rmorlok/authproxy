# Migrations

## ToolSet logical storage

Migration 17 adds `tool_sets`, the logical identity and connection-selection
policy shared by a ToolSet's generations. Live names are unique within a
namespace; soft deletion releases the name while retaining the ID.

`connection_selector` stores the canonical selector JSON, preserving an explicit
empty `matchLabels` map. `selector_revision` starts at one and changes atomically
with each accepted selector replacement. It is independent of generation
numbers and lets membership work detect stale selection policies. Namespace
label propagation changes metadata without changing this revision.

This table contains no executable definitions or encrypted source artifacts.
Generation storage and its encryption registration follow separately, as do
binding cleanup, namespace-deletion/purge integration, and management APIs.

## Creating and running migrations

AuthProxy uses [golang-migrate](https://github.com/golang-migrate/migrate) for migrations. To add a new migration,
use the following command:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate create -dir internal/database/migrations/sqlite -seq -ext .sql <migration_name>
```

For Postgres:

```bash
go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate create -dir internal/database/migrations/postgres -seq -ext .sql <migration_name>
```

to test migrations:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate -source file://internal/database/migrations/sqlite -database sqlite3://tmp/dev.db up
```

```bash
go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate -source file://internal/database/migrations/postgres -database "postgres://localhost:5432/authproxy?sslmode=disable" up
```

to rollback a migration:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate/sqlite -source file://internal/database/migrations -database sqlite3://tmp/dev.db down 1
```

```bash
go run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate/postgres -source file://internal/database/migrations -database "postgres://localhost:5432/authproxy?sslmode=disable" down 1
```
