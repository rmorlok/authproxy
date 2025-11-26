package database

import (
	"context"
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/pkg/errors"
)

//go:embed migrations/**/*.sql
var migrationsFs embed.FS

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "db-migrate-lock"

func (db *gormDB) Migrate(ctx context.Context) error {
	db.logger.Info("running database migrations", "provider", db.cfg.GetProvider())
	defer db.logger.Info("database migrations complete")

	d, err := iofs.New(migrationsFs, fmt.Sprintf("migrations/%s", db.cfg.GetProvider()))
	if err != nil {
		return errors.Wrapf(err, "failed to load databse migrations for '%s'", db.cfg.GetProvider())
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, db.cfg.GetUri())
	if err != nil {
		return errors.Wrap(err, "failed setup database migrations")
	}

	err = m.Up()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			db.logger.Info("no migrations required")
			return nil
		}

		return errors.Wrap(err, "failed to migrate database")
	}

	return nil
}
