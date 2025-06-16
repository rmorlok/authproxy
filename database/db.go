package database

import (
	"context"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/jwt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"log/slog"
	"os"
	"time"
)

//go:generate mockgen -source=./db.go -destination=./mock/db.go -package=mock
type DB interface {
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) bool

	/*
	 *  Actors
	 */

	GetActor(ctx context.Context, id uuid.UUID) (*Actor, error)
	GetActorByExternalId(ctx context.Context, externalId string) (*Actor, error)
	CreateActor(ctx context.Context, actor *Actor) error
	UpsertActor(ctx context.Context, actor *jwt.Actor) (*Actor, error)
	ListActorsBuilder() ListActorsBuilder
	ListActorsFromCursor(ctx context.Context, cursor string) (ListActorsExecutor, error)

	/*
	 * Connectors
	 */

	GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForTypeAndVersion(ctx context.Context, typ string, version uint64) (*ConnectorVersion, error)
	GetConnectorVersionForType(ctx context.Context, typ string) (*ConnectorVersion, error)
	GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state ConnectorVersionState) (*ConnectorVersion, error)
	NewestConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	NewestPublishedConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error)
	UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error
	ListConnectorsBuilder() ListConnectorsBuilder
	ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error)

	/*
	 *  Connections
	 */

	GetConnection(ctx context.Context, id uuid.UUID) (*Connection, error)
	CreateConnection(ctx context.Context, c *Connection) error
	ListConnectionsBuilder() ListConnectionsBuilder
	ListConnectionsFromCursor(ctx context.Context, cursor string) (ListConnectionsExecutor, error)

	/*
	 * OAuth2 tokens
	 */
	GetOAuth2Token(ctx context.Context, connectionId uuid.UUID) (*OAuth2Token, error)
	InsertOAuth2Token(
		ctx context.Context,
		connectionId uuid.UUID,
		refreshedFrom *uuid.UUID,
		encryptedRefreshToken string,
		encryptedAccessToken string,
		accessTokenExpiresAt *time.Time,
		scopes string,
	) (*OAuth2Token, error)

	// EnumerateOAuth2TokensExpiringWithin enumerates OAuth2 tokens that are expiring within a specified time interval
	// of now. This includes tokens that are already expired. Deleted tokens are not considered, nor are tokens tied
	// to a deleted connection.
	EnumerateOAuth2TokensExpiringWithin(
		ctx context.Context,
		duration time.Duration,
		callback func(tokens []*OAuth2TokenWithConnection, lastPage bool) (stop bool, err error),
	) error

	/*
	 *  Nonces
	 */

	HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (hasBeenUsed bool, err error)
	CheckNonceValidAndMarkUsed(ctx context.Context, nonce uuid.UUID, retainRecordUntil time.Time) (wasValid bool, err error)
}

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
