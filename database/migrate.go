package database

import (
	"context"
	"github.com/pkg/errors"
)

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "db-migrate-lock"

func (db *gormDB) Migrate(ctx context.Context) error {
	err := db.gorm.AutoMigrate(&Actor{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate actors")
	}

	err = db.gorm.AutoMigrate(&ConnectorVersion{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate connector versions")
	}

	err = db.gorm.AutoMigrate(&Connection{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate connections")
	}

	err = db.gorm.AutoMigrate(&UsedNonce{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate used nonces")
	}

	err = db.gorm.AutoMigrate(&OAuth2Token{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate oauth2 tokens")
	}

	return nil
}
