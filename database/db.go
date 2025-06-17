package database

import (
	"context"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"log/slog"
	"os"
	"time"
)

// NewConnectionForRoot creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration. Same as NewConnection.
func NewConnectionForRoot(root *config.Root, logger *slog.Logger) (DB, error) {
	dbConfig := root.Database
	secretKey := root.SystemAuth.GlobalAESKey

	switch dbConfig.(type) {
	case *config.DatabaseSqlite:
		return NewSqliteConnection(dbConfig.(*config.DatabaseSqlite), secretKey, logger)
	default:
		return nil, errors.New("database type not supported")
	}
}

// NewSqliteConnection creates a new database connection to a SQLite database.
//
// Parameters:
// - dbConfig: the configuration for the SQLite database
// - secretKey: the AES key used to secure cursors
func NewSqliteConnection(dbConfig *config.DatabaseSqlite, secretKey config.KeyData, l *slog.Logger) (DB, error) {
	path := dbConfig.Path
	_, err := os.Stat(path)
	if err != nil {
		// attempt home path expansion
		path, err = homedir.Expand(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to expand path; could not load sqlite database path '%s'", dbConfig.Path)
		}
	}

	_, err = os.Stat(path)
	if err != nil {
		// Attempt to create file
		file, err := os.Create(path)
		if err != nil {
			return nil, errors.Wrapf(err, "could not load sqlite database path '%s'; failed to create", dbConfig.Path)
		}
		defer file.Close()
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: &logger{
			inner: l,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite database '%s'", dbConfig.Path)
	}

	return &gormDB{gorm: db, secretKey: secretKey, logger: l}, nil
}

type gormDB struct {
	gorm      *gorm.DB       // the gorm instance
	secretKey config.KeyData // the AES key used to secure cursors
	logger    *slog.Logger
}

func (db *gormDB) session(ctx context.Context) *gorm.DB {
	return db.gorm.Session(&gorm.Session{
		NowFunc: func() time.Time {
			return apctx.GetClock(ctx).Now().UTC()
		},
	})
}

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

func (db *gormDB) Ping(ctx context.Context) bool {
	err := db.session(ctx).Raw("SELECT 1").Error
	if err != nil {
		log.Println(errors.Wrap(err, "failed to connect to database"))
		return false
	}

	return true
}

var _ DB = (*gormDB)(nil)
