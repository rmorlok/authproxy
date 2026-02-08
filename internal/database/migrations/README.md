# Migrations

AuthProxy uses [golang-migrate](github.com/golang-migrate/migrate) for migrations. To add a new migration,
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
