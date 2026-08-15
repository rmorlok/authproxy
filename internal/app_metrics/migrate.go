package app_metrics

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	chmigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// MigrationStatus returns the migration status for the app metrics database.
func MigrationStatus(ctx context.Context, cfg *config.Database) migration.Status {
	if cfg == nil || cfg.InnerVal == nil {
		return migration.UnavailableStatus(
			migration.TargetAppMetrics,
			"",
			0,
			fmt.Errorf("app metrics database configuration is required"),
		)
	}

	provider := cfg.GetProvider()
	latest, err := migration.LatestVersion(
		appMetricsMigrationsFs,
		fmt.Sprintf("migrations/%s", provider),
	)
	if err != nil {
		return migration.UnavailableStatus(migration.TargetAppMetrics, provider, 0, err)
	}

	return migration.Inspect(
		ctx,
		migration.TargetAppMetrics,
		cfg,
		appMetricsMigrationsTable,
		latest,
	)
}

// RunMigrations runs any necessary schema migrations for the app metrics
// database. This will run on clickhouse, postgres, or sqlite depending on
// the provider in the configuration.
func RunMigrations(
	ctx context.Context,
	cfg *config.Database,
	logger *slog.Logger,
	direction migration.Direction,
	version *uint,
) (resultErr error) {
	if cfg == nil || cfg.InnerVal == nil {
		return fmt.Errorf("app metrics database configuration is required")
	}

	provider := cfg.GetProvider()
	logger.Info(
		"running app metrics database migrations",
		"provider", provider,
		"direction", direction,
		"version", version,
	)

	// Deferred logging based on error status
	defer func() {
		if resultErr != nil {
			logger.Error(
				"app metrics database migrations failed",
				"provider", provider,
				"error", resultErr,
			)
			return
		}
		logger.Info("app metrics database migrations complete", "provider", provider)
	}()

	// Migrations are read from the embedded filesystem
	source, err := iofs.New(appMetricsMigrationsFs, fmt.Sprintf("migrations/%s", provider))
	if err != nil {
		return fmt.Errorf("load app metrics migrations for %q: %w", provider, err)
	}

	// Get a go-migrate driver that is provider specific
	driver, driverName, err := newMigrationDriver(ctx, cfg)
	if err != nil {
		return fmt.Errorf("setup app metrics migration driver: %w", err)
	}

	// Initialize a migrator that pulls from the embedded fs
	migrator, err := migrate.NewWithInstance("iofs", source, driverName, driver)
	if err != nil {
		_ = driver.Close()
		return fmt.Errorf("setup app metrics migrator: %w", err)
	}

	// Defer cleanup and logging
	defer func() {
		sourceErr, dbErr := migrator.Close()
		if sourceErr != nil || dbErr != nil {
			logger.Warn(
				"failed to close app metrics migrator",
				"source_err", sourceErr,
				"db_err", dbErr,
			)
		}
	}()

	// Run the migration.
	if err := migration.Apply(ctx, migrator, direction, version); err != nil {
		return fmt.Errorf("run app metrics migrations: %w", err)
	}

	return nil
}

// newMigrationDriver creates a new migration driver based on the provided
// configuration. This is mapping cfg options to the options passed to the
// go-migrate driver and returning an initialized object.
func newMigrationDriver(
	ctx context.Context,
	cfg *config.Database,
) (migratedb.Driver, string, error) {
	switch concrete := cfg.InnerVal.(type) {

	case *config.DatabasePostgres:
		db, err := sql.Open(concrete.GetDriver(), concrete.GetDsn())
		if err != nil {
			return nil, "", err
		}
		driver, err := postgres.WithInstance(
			db,
			&postgres.Config{
				MigrationsTable: appMetricsMigrationsTable,
			},
		)
		if err != nil {
			_ = db.Close()
		}
		return driver, "postgres", err

	case *config.DatabaseSqlite:
		db, err := sql.Open(concrete.GetDriver(), concrete.GetDsn())
		if err != nil {
			return nil, "", err
		}
		driver, err := sqlite3.WithInstance(
			db,
			&sqlite3.Config{
				MigrationsTable: appMetricsMigrationsTable,
			},
		)
		if err != nil {
			_ = db.Close()
		}
		return driver, "sqlite", err

	case *config.DatabaseClickhouse:
		opts, err := concrete.ToClickhouseOptions()
		if err != nil {
			return nil, "", err
		}
		dbName := ""
		if concrete.Database != nil {
			dbName, err = concrete.Database.GetValue(ctx)
			if err != nil {
				return nil, "", err
			}
		}
		db := sql.OpenDB(clickhouse.Connector(opts))
		driver, err := chmigrate.WithInstance(db, &chmigrate.Config{
			DatabaseName:          dbName,
			MigrationsTable:       appMetricsMigrationsTable,
			MultiStatementEnabled: true,
		})
		if err != nil {
			_ = db.Close()
		}
		return driver, "clickhouse", err

	default:
		return nil, "", fmt.Errorf("unsupported app metrics provider %q", cfg.GetProvider())
	}
}
