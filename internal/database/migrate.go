package database

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/pkg/errors"
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
		return errors.Wrapf(err, "failed to load database migrations for '%s'", s.cfg.GetProvider())
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, s.cfg.GetUri())
	if err != nil {
		return errors.Wrap(err, "failed setup database migrations")
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

		return errors.Wrap(err, "failed to migrate database")
	}

	return nil
}
