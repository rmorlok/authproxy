package database

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"os"
	"time"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

	db, err := sql.Open("sqlite3", dbConfig.GetDsn())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite database '%s'", dbConfig.GetDsn())
	}

	if err := db.Ping(); err != nil {
		return nil, errors.Wrapf(err, "failed to ping sqlite database '%s'", dbConfig.GetDsn())
	}

	gormDb, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: &logger{
			inner: l,
		},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite database '%s' with gorm", dbConfig.Path)
	}

	return &service{
		cfg:       dbConfig,
		sq:        sq.StatementBuilder.PlaceholderFormat(dbConfig.GetPlaceholderFormat()),
		db:        db,
		gorm:      gormDb,
		secretKey: secretKey,
		logger:    l,
	}, nil
}

type service struct {
	cfg       config.Database
	sq        sq.StatementBuilderType
	db        *sql.DB
	gorm      *gorm.DB       // the gorm instance
	secretKey config.KeyData // the AES key used to secure cursors
	logger    *slog.Logger
}

func (s *service) session(ctx context.Context) *gorm.DB {
	return s.gorm.Session(&gorm.Session{
		NowFunc: func() time.Time {
			return apctx.GetClock(ctx).Now().UTC()
		},
	})
}

func (s *service) Ping(ctx context.Context) bool {
	if err := s.db.Ping(); err != nil {
		s.logger.Error("failed to ping database")
		return false
	}

	err := s.session(ctx).Raw("SELECT 1").Error
	if err != nil {
		s.logger.Error("failed to ping database with query")
		log.Println(errors.Wrap(err, "failed to connect to database"))
		return false
	}

	return true
}

var _ DB = (*service)(nil)
