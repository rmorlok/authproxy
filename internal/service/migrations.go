package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/tasks"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/migration"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/workflows"
)

func (dm *DependencyManager) MigrationStatuses(
	ctx context.Context,
	target migration.Target,
) []migration.Status {
	if target == migration.TargetAll {
		statuses := make([]migration.Status, 0, len(migration.OrderedTargets))
		for _, currentTarget := range migration.OrderedTargets {
			statuses = append(statuses, dm.migrationStatus(ctx, currentTarget))
		}
		return statuses
	}

	return []migration.Status{dm.migrationStatus(ctx, target)}
}

func (dm *DependencyManager) migrationStatus(
	ctx context.Context,
	target migration.Target,
) migration.Status {
	root := dm.GetConfigRoot()

	switch target {
	case migration.TargetMainDatabase:
		return database.MigrationStatus(ctx, root.Database)

	case migration.TargetWorkflows:
		return workflows.MigrationStatus(ctx, root)

	case migration.TargetAppMetrics:
		if root.AppMetrics == nil {
			return migration.UnavailableStatus(
				target,
				"", // provider
				0,  // available
				fmt.Errorf("app metrics configuration is required"),
			)
		}
		return app_metrics.MigrationStatus(ctx, root.AppMetrics.Database)

	default:
		return migration.UnavailableStatus(
			target,
			"", // provider
			0,  // available
			fmt.Errorf("unknown migration database %q", target),
		)
	}
}

func (dm *DependencyManager) VerifyMigrations(ctx context.Context) error {
	var result error
	for _, status := range dm.MigrationStatuses(ctx, migration.TargetAll) {
		if err := migration.IncompatibleError(status); err != nil {
			result = errors.Join(result, err)
		}
	}

	if result != nil {
		return fmt.Errorf("schema compatibility verification failed: %w", result)
	}

	return nil
}

func (dm *DependencyManager) MigrateSchemas(
	ctx context.Context,
	target migration.Target,
	direction migration.Direction,
	version *uint,
) error {
	if target == migration.TargetAll && version != nil {
		return fmt.Errorf("a schema version cannot be used with all because each database has an independent version sequence")
	}

	if target != migration.TargetAll {
		return dm.migrateTargetWithLock(ctx, target, direction, version)
	}

	//
	// Migrate all targets in the correct order.
	//

	if direction == migration.DirectionDown {
		// Reversed order from up direction

		// metrics
		if err := dm.migrateTargetWithLock(
			ctx,
			migration.TargetAppMetrics,
			direction, // down
			nil,       // version
		); err != nil {
			return err
		}

		return dm.withMainDatabaseMigrationLock(ctx, func(ctx context.Context) error {
			// workflows database (part of main database)
			if err := dm.migrateTarget(
				ctx,
				migration.TargetWorkflows,
				direction, // down
				nil,       // version
			); err != nil {
				return err
			}

			// main database
			return dm.migrateTarget(
				ctx,
				migration.TargetMainDatabase,
				direction, // down
				nil,       // version
			)
		})
	}

	if err := dm.withMainDatabaseMigrationLock(
		ctx,
		func(ctx context.Context) error {
			// main database
			if err := dm.migrateTarget(
				ctx,
				migration.TargetMainDatabase,
				direction, // up
				nil,       // version
			); err != nil {
				return err
			}

			// workflows database (part of main database)
			return dm.migrateTarget(
				ctx,
				migration.TargetWorkflows,
				direction, // up
				nil,       // version
			)
		},
	); err != nil {
		return err
	}

	// metrics database
	return dm.migrateTargetWithLock(
		ctx,
		migration.TargetAppMetrics,
		direction, // up
		nil,       // version
	)
}

func (dm *DependencyManager) migrateTargetWithLock(
	ctx context.Context,
	target migration.Target,
	direction migration.Direction,
	version *uint,
) error {
	switch target {
	case migration.TargetMainDatabase, migration.TargetWorkflows:
		return dm.withMainDatabaseMigrationLock(ctx, func(ctx context.Context) error {
			return dm.migrateTarget(ctx, target, direction, version)
		})

	case migration.TargetAppMetrics:
		root := dm.GetConfigRoot()
		lockDuration := root.AppMetrics.Database.GetAutoMigrationLockDuration()
		return dm.withMigrationLock(ctx, app_metrics.MigrateMutexKeyName, lockDuration, func(ctx context.Context) error {
			return dm.migrateTarget(ctx, target, direction, version)
		})

	default:
		return fmt.Errorf("unknown migration database %q", target)
	}
}

func (dm *DependencyManager) withMainDatabaseMigrationLock(
	ctx context.Context,
	fn func(context.Context) error,
) error {
	return dm.withMigrationLock(
		ctx,
		database.MigrateMutexKeyName,
		dm.GetConfigRoot().Database.GetAutoMigrationLockDuration(),
		fn,
	)
}

func (dm *DependencyManager) withMigrationLock(
	ctx context.Context,
	key string,
	duration time.Duration,
	fn func(context.Context) error,
) error {
	redisClient, err := dm.GetRedisClientWithError()
	if err != nil {
		return fmt.Errorf("connect to redis for migration lock %q: %w", key, err)
	}
	mutex := apredis.NewMutex(
		redisClient,
		key,
		apredis.MutexOptionLockFor(duration),
		apredis.MutexOptionRetryFor(duration+time.Second),
		apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)

	if err := apredis.RunWithMutex(ctx, mutex, duration, fn); err != nil {
		return fmt.Errorf("migration lock %q: %w", key, err)
	}

	return nil
}

func (dm *DependencyManager) migrateTarget(
	ctx context.Context,
	target migration.Target,
	direction migration.Direction,
	version *uint,
) error {
	status := dm.migrationStatus(ctx, target)
	if err := migration.ValidateRequest(status, direction, version); err != nil {
		return fmt.Errorf("validate %s migration: %w", target, err)
	}

	logBuilder, err := dm.GetLogBuilderWithError()
	if err != nil {
		return err
	}

	logger := logBuilder.WithComponent("migrations").Build()

	started := time.Now()

	requestedVersion := "latest"
	if direction == migration.DirectionDown && version == nil {
		requestedVersion = "previous"
	} else if version != nil {
		requestedVersion = fmt.Sprint(*version)
	}

	logger.Info(
		"starting schema migration",
		"target", target,
		"provider", status.Provider,
		"current_version", status.CurrentVersionString(),
		"available_version", status.AvailableVersion,
		"direction", direction,
		"requested_version", requestedVersion,
	)

	err = nil
	switch target {
	case migration.TargetMainDatabase:
		err = database.RunMigrations(ctx, dm.GetConfigRoot().Database, logger, direction, version)
	case migration.TargetWorkflows:
		err = workflows.RunMigrations(ctx, dm.GetConfigRoot(), logger, direction, version)
	case migration.TargetAppMetrics:
		err = app_metrics.RunMigrations(ctx, dm.GetConfigRoot().AppMetrics.Database, logger, direction, version)
	default:
		err = fmt.Errorf("unknown migration database %q", target)
	}
	if err != nil {
		return fmt.Errorf("migrate %s: %w", target, err)
	}

	result := dm.migrationStatus(ctx, target)
	if result.Err != nil {
		return fmt.Errorf("inspect %s after migration: %w", target, result.Err)
	}

	if version != nil && (result.CurrentVersion == nil || *result.CurrentVersion != *version) {
		return fmt.Errorf(
			"%s migration finished at version %s instead of requested version %d",
			target, result.CurrentVersionString(), *version)
	}

	if direction == migration.DirectionUp && version == nil && !result.Compatible() {
		return migration.IncompatibleError(result)
	}

	logger.Info(
		"schema migration complete",
		"target", target,
		"current_version", result.CurrentVersionString(),
		"available_version", result.AvailableVersion,
		"duration", time.Since(started),
	)

	return nil
}

func (dm *DependencyManager) RunProductionMigration(
	ctx context.Context,
	target migration.Target,
	direction migration.Direction,
	version *uint,
) error {
	if err := dm.MigrateSchemas(ctx, target, direction, version); err != nil {
		return err
	}
	if direction != migration.DirectionUp || (target != migration.TargetAll && target != migration.TargetMainDatabase) {
		return nil
	}
	mainStatus := dm.migrationStatus(ctx, migration.TargetMainDatabase)
	if !mainStatus.Compatible() {
		return nil
	}
	return dm.BootstrapEncryption(ctx)
}

func (dm *DependencyManager) BootstrapEncryption(ctx context.Context) error {
	db, err := dm.GetDatabaseWithError()
	if err != nil {
		return fmt.Errorf("open main database for encryption bootstrap: %w", err)
	}
	// The root namespace is runtime bootstrap data rather than schema or
	// configured-resource reconciliation. A newly migrated database needs it
	// before encryption keys can be associated with namespaces.
	if err := db.EnsureNamespaceByPath(ctx, namespace.Root); err != nil {
		return fmt.Errorf("ensure root namespace: %w", err)
	}
	redisClient, err := dm.GetRedisClientWithError()
	if err != nil {
		return fmt.Errorf("connect to redis for encryption bootstrap: %w", err)
	}
	logger, err := dm.GetLoggerWithError()
	if err != nil {
		return err
	}
	keyTelemetry, err := dm.GetDataEncryptionKeyTelemetryWithError()
	if err != nil {
		return err
	}
	if err := encrypt.GenerateDataEncryptionKeysToDatabase(
		ctx,
		dm.GetConfig(),
		db,
		logger,
		redisClient,
		encrypt.WithGenerateDataEncryptionKeysTelemetry(keyTelemetry),
	); err != nil {
		return fmt.Errorf("generate data encryption keys: %w", err)
	}
	if err := encrypt.SyncKeysToDatabase(
		ctx,
		dm.GetConfig(),
		db,
		logger,
		redisClient,
		encrypt.WithSyncKeysTelemetry(keyTelemetry),
	); err != nil {
		return fmt.Errorf("sync encryption keys: %w", err)
	}
	return nil
}

func (dm *DependencyManager) RunDevelopmentMigration(ctx context.Context) error {
	if err := dm.RunProductionMigration(ctx, migration.TargetAll, migration.DirectionUp, nil); err != nil {
		return err
	}
	if err := dm.ReconcileDevelopmentData(ctx); err != nil {
		return err
	}
	return dm.VerifyMigrations(ctx)
}

func (dm *DependencyManager) ReconcileDevelopmentData(ctx context.Context) error {
	if err := dm.reconcileConfiguredConnectors(ctx); err != nil {
		return err
	}
	if err := dm.reconcileConfiguredActors(ctx); err != nil {
		return err
	}
	return nil
}

func (dm *DependencyManager) reconcileConfiguredConnectors(ctx context.Context) error {
	lockDuration := dm.GetConfigRoot().Connectors.GetAutoMigrationLockDurationOrDefault()
	return dm.withMigrationLock(ctx, core.MigrateMutexKeyName, lockDuration, func(ctx context.Context) error {
		if err := dm.GetCoreService().Migrate(ctx); err != nil {
			return fmt.Errorf("reconcile configured connectors: %w", err)
		}
		return nil
	})
}

func (dm *DependencyManager) reconcileConfiguredActors(ctx context.Context) error {
	actors := dm.GetConfigRoot().SystemAuth.Actors
	if actors == nil {
		return nil
	}
	switch actors.InnerVal.(type) {
	case *sconfig.ConfiguredActorsExternalSources:
		if _, err := dm.GetAsyncClient().Enqueue(tasks.NewSyncActorsExternalSourceTask()); err != nil {
			return fmt.Errorf("enqueue configured actor synchronization: %w", err)
		}
		return nil
	}
	if _, ok := actors.InnerVal.(sconfig.ConfiguredActorsList); !ok {
		return nil
	}

	const lockDuration = 30 * time.Second
	return dm.withMigrationLock(ctx, "actor_sync:migrate", lockDuration, func(ctx context.Context) error {
		db, err := dm.GetDatabaseWithError()
		if err != nil {
			return err
		}
		redisClient, err := dm.GetRedisClientWithError()
		if err != nil {
			return err
		}
		svc := tasks.NewService(dm.GetConfig(), db, redisClient, dm.GetEncryptService(), dm.GetLogger())
		return svc.SyncActorList(ctx)
	})
}

func (dm *DependencyManager) ShutdownMigrationResources() {
	if dm.e != nil {
		dm.e.Shutdown()
	}
	if dm.asynqClient != nil {
		_ = dm.asynqClient.Close()
		dm.asynqClient = nil
	}
	if dm.asynqInspector != nil {
		_ = dm.asynqInspector.Close()
		dm.asynqInspector = nil
	}
	dm.ShutdownWorkflowRuntime()
	dm.ShutdownDatabase()
	if dm.r != nil {
		_ = dm.r.Close()
		dm.r = nil
	}
	dm.ShutdownTelemetry()
}
