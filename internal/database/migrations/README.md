# Migrations

AuthProxy uses [gormigrate](https://github.com/go-gormigrate/gormigrate) for migrations. To add a new migration,
use the following command:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate create -dir internal/database/migrations/sqlite -seq -ext .sql <migration_name>
```

to test migrations:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate -source file://internal/database/migrations/sqlite -database sqlite3://tmp/dev.db up
```

to rollback a migration:

```bash
go run -tags sqlite3 github.com/golang-migrate/migrate/v4/cmd/migrate/sqlite -source file://internal/database/migrations -database sqlite3://tmp/dev.db down 1
```
