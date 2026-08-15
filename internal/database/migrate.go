package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/rmorlok/authproxy/internal/migration"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

//go:embed migrations/**/*.sql
var migrationsFs embed.FS

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "db-migrate-lock"

const migrationsTable = "schema_migrations"

func MigrationStatus(ctx context.Context, cfg *config.Database) migration.Status {
	if cfg == nil || cfg.InnerVal == nil {
		return migration.UnavailableStatus(migration.TargetMainDatabase, "", 0, fmt.Errorf("database configuration is required"))
	}
	latest, err := migration.LatestVersion(migrationsFs, fmt.Sprintf("migrations/%s", cfg.GetProvider()))
	if err != nil {
		return migration.UnavailableStatus(migration.TargetMainDatabase, cfg.GetProvider(), 0, err)
	}
	return migration.Inspect(ctx, migration.TargetMainDatabase, cfg, migrationsTable, latest)
}

func (s *service) Migrate(ctx context.Context) (resultErr error) {
	return RunMigrations(ctx, &config.Database{InnerVal: s.cfg}, s.logger, migration.DirectionUp, nil)
}

func RunMigrations(
	ctx context.Context,
	cfg *config.Database,
	logger *slog.Logger,
	direction migration.Direction,
	version *uint,
) (resultErr error) {
	logger.Info("running database migrations", "provider", cfg.GetProvider(), "direction", direction, "version", version)
	defer func() {
		if resultErr != nil {
			logger.Error("database migrations failed", "provider", cfg.GetProvider(), "error", resultErr)
			return
		}
		logger.Info("database migrations complete", "provider", cfg.GetProvider())
	}()

	d, err := iofs.New(migrationsFs, fmt.Sprintf("migrations/%s", cfg.GetProvider()))
	if err != nil {
		return fmt.Errorf("failed to load database migrations for '%s': %w", cfg.GetProvider(), err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, cfg.GetUri())
	if err != nil {
		return fmt.Errorf("failed setup database migrations: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			logger.Warn("failed to close migrator", "source_err", sourceErr, "db_err", dbErr)
		}
	}()

	err = migration.Apply(ctx, m, direction, version)
	if err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}
