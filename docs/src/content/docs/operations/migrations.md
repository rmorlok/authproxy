---
title: Database migrations
description: Inspect and migrate AuthProxy schemas safely before starting production services.
---

AuthProxy verifies every configured schema before opening a listener or starting
a worker. Normal `serve` processes never change schemas. Run migrations as an
explicit deployment step before starting or updating production workloads.

## Inspect schema status

Report every migration target without changing a database:

```sh
authproxy migrate status --config=/etc/authproxy/config.yaml
```

Limit the report to one target by adding `main-database`, `workflows`, or
`app-metrics`:

```sh
authproxy migrate status app-metrics --config=/etc/authproxy/config.yaml
```

The report includes the provider, current version, highest version available in
the AuthProxy binary, dirty flag, and compatibility state. The command exits
non-zero when a requested target is missing, behind, ahead, dirty, or
unavailable. Inspection is read-only and does not create a missing SQLite file
or migration table.

## Apply migrations

Upgrade every target to its latest version in dependency order:

```sh
authproxy migrate all --config=/etc/authproxy/config.yaml
```

The order is `main-database`, `workflows`, then `app-metrics`. Processing stops
at the first failure. Earlier targets remain migrated because independent
databases cannot share a transaction.

Target one schema or an exact version when needed:

```sh
authproxy migrate main-database
authproxy migrate main-database up 16
authproxy migrate workflows down
authproxy migrate workflows down 2
```

`up` is the default direction. Without a version, `up` selects the latest
available version and `down` rolls back one version. A supplied version is an
absolute target version. Exact versions are not accepted with `all`, because
the three targets have independent version sequences. `all down` rolls back
one version in reverse dependency order: app metrics, workflows, then the main
database.

AuthProxy does not automatically force or repair a dirty schema. Diagnose the
failed migration and restore or repair the database deliberately before
retrying.

## Local automatic migration

For a disposable development environment, migration and configuration
reconciliation can run once before service fan-out:

```sh
go run ./cmd/server serve --auto-migrate \
  --config=./dev_config/default.yaml all
```

`--auto-migrate` is intentionally a CLI-only option and is unsafe for
production. It upgrades all schemas, initializes required encryption-key
state, reconciles configured connectors and actors, verifies compatibility,
and only then starts the requested services. Omitting it performs read-only
verification only.

## Locks and concurrency

The main and workflow schemas share the renewable Redis lock
`db-migrate-lock`; app metrics uses `app-metrics-migrate-lock`. The lock lease
is refreshed every third of its configured duration. PostgreSQL migration
drivers also retain their native advisory lock. Concurrent migration processes
therefore serialize across SQLite, PostgreSQL, and ClickHouse while retaining
the database-native protection available in PostgreSQL.

`database.autoMigrationLockDuration`,
`appMetrics.database.autoMigrationLockDuration`, and
`connectors.autoMigrationLockDuration` tune lock leases. They do not enable
automatic migration.

## Kubernetes deployments

The Helm chart and demo Kustomize manifests include a dedicated migration Job
that uses the same image, configuration, credentials, and mounted key material
as the AuthProxy workload. Wait for that Job to succeed before waiting for or
promoting the AuthProxy Deployment. A failed Job stops the rollout and its pod
logs contain the target-specific migration error.

For Helm, use `--wait --wait-for-jobs`; every release revision renders a new
migration Job. Application pods may be scheduled concurrently, but mandatory
startup verification prevents them from opening listeners or starting workers
until the Job has made every schema current.
