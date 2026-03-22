package database

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/**/*.sql
var migrationsFs embed.FS

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "db-migrate-lock"

func (s *service) Migrate(ctx context.Context) error {
	s.logger.Info("running database migrations", "provider", s.cfg.GetProvider())
	defer s.logger.Info("database migrations complete")

	d, err := iofs.New(migrationsFs, fmt.Sprintf("migrations/%s", s.cfg.GetProvider()))
	if err != nil {
		return fmt.Errorf("failed to load database migrations for '%s': %w", s.cfg.GetProvider(), err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, s.cfg.GetUri())
	if err != nil {
		return fmt.Errorf("failed setup database migrations: %w", err)
	}
	defer func() {
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			s.logger.Warn("failed to close migrator", "source_err", sourceErr, "db_err", dbErr)
		}
	}()

	err = m.Up()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			s.logger.Info("no migrations required")
			return nil
		}

		return fmt.Errorf("failed to migrate database: %w", err)
	}

	return nil
}
