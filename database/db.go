package database

import (
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"os"
	"time"
)

type DB interface {
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) bool

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	/*
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce uuid.UUID, retainRecordUntil time.Time) (wasValid bool, err error)
}

// NewConnection creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration.
func NewConnection(c config.C) (DB, error) {
	return NewConnectionForRoot(c.GetRoot())
}

// NewConnectionForRoot creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration. Same as NewConnection.
func NewConnectionForRoot(root *config.Root) (DB, error) {
	dbConfig := root.Database
	secretKey := root.SystemAuth.GlobalAESKey

	switch dbConfig.(type) {
	case *config.DatabaseSqlite:
		return NewSqliteConnection(dbConfig.(*config.DatabaseSqlite), secretKey)
	default:
		return nil, errors.New("database type not supported")
	}
}

// NewSqliteConnection creates a new database connection to a SQLite database.
//
// Parameters:
// - dbConfig: the configuration for the SQLite database
// - secretKey: the AES key used to secure cursors
func NewSqliteConnection(dbConfig *config.DatabaseSqlite, secretKey config.KeyData) (DB, error) {
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

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite database '%s'", dbConfig.Path)
	}

	return &gormDB{gorm: db, secretKey: secretKey}, nil
}

type gormDB struct {
	gorm      *gorm.DB       // the gorm instance
	secretKey config.KeyData // the AES key used to secure cursors
}

func (db *gormDB) session(ctx context.Context) *gorm.DB {
	return db.gorm.Session(&gorm.Session{
		NowFunc: func() time.Time {
			return ctx.Clock().Now().UTC()
		},
	})
}

func (db *gormDB) Migrate(ctx context.Context) error {
	err := db.gorm.AutoMigrate(&Actor{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate actors")
	}

	err = db.gorm.AutoMigrate(&Connection{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate connections")
	}

	err = db.gorm.AutoMigrate(&UsedNonce{})
	if err != nil {
		return errors.Wrap(err, "failed to auto migrate used nonces")
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
