---
title: Automatic migrations
description: Understand how AuthProxy coordinates schema migrations and startup reconciliation across service processes.
---

AuthProxy can apply database migrations automatically when a service starts.
Every enabled service may start at the same time, so automatic migration uses
Redis mutexes to ensure that only one process migrates each storage target.

## Configure automatic migration

The primary database and application-metrics store have independent switches
and independent locks:

~~~yaml
database:
  provider: postgres
  autoMigrate: true
  autoMigrationLockDuration: 2m
  # ...

app_metrics:
  autoMigrate: true
  database:
    provider: clickhouse
    autoMigrationLockDuration: 2m
    # ...
~~~

The `autoMigrationLockDuration` value is the Redis lease duration.
AuthProxy refreshes the lease while migration is running, so it is not a
maximum migration runtime. Contending processes wait for a bounded period
based on the same duration and then fail startup rather than continuing
without exclusivity.

## Lock targets and providers

The primary relational schema and workflow schema share
`db-migrate-lock` and run sequentially. The application-metrics
schema uses the separate `app-metrics-migrate-lock`, because it may
live in a different database and should not block an unrelated primary
migration.

The Redis contract is the same for SQLite, PostgreSQL, and ClickHouse:

1. acquire the target mutex;
2. refresh it every third of the lease duration;
3. apply all migrations;
4. release the mutex and report any release failure.

If a refresh fails, AuthProxy cancels the migration context, asks the migration
runner to stop at its next safe boundary, and fails startup. It never treats an
expired or unconfirmed lease as successful migration ownership.

PostgreSQL also retains the advisory lock supplied by its migration driver.
That database-native lock provides defense in depth and interoperability with
other processes that use the same migration driver. Redis supplies the
provider-independent coordination needed by SQLite and ClickHouse.

## Startup reconciliation

Schema migration is followed by reconciliation of encryption keys, configured
connectors, namespaces, and configured actors. These operations use distinct
renewable Redis mutexes because they may include database cleanup or calls to
external key providers. A key-sync sentinel additionally suppresses redundant
work; the sentinel controls frequency, while the mutex prevents overlap.

Failures acquiring, refreshing, or releasing these mutexes are surfaced as
startup or task errors; automatic database migration logs also identify its
lock key. Successful database migration, no-change outcomes, and original
migration failures are logged separately.

## Production rollout

Automatic migration is convenient for local, demo, and controlled deployments.
For a multi-replica production rollout, keep database backups current and
ensure all replicas use the same Redis service. Prefer a deployment workflow
that runs migration as an explicit pre-rollout step and starts application
replicas only after it succeeds; dedicated migration-job packaging is tracked
separately from the automatic startup path.
