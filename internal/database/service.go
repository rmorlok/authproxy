package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mitchellh/go-homedir"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// NewConnectionForRoot creates a new database connection from the specified configuration. The type of the database
// returned will be determined by the configuration. Same as NewConnection.
func NewConnectionForRoot(root *config.Root, logger *slog.Logger) (DB, error) {
	switch v := root.Database.InnerVal.(type) {
	case *config.DatabaseSqlite:
		return NewSqliteConnection(v, logger)
	case *config.DatabasePostgres:
		return NewPostgresConnection(v, logger)
	default:
		return nil, errors.New("database type not supported")
	}
}

// NewSqliteConnection creates a new database connection to a SQLite database.
func NewSqliteConnection(dbConfig *config.DatabaseSqlite, l *slog.Logger) (DB, error) {
	path := dbConfig.Path
	_, err := os.Stat(path)
	if err != nil {
		// attempt home path expansion
		path, err = homedir.Expand(path)
		if err != nil {
			return nil, fmt.Errorf("failed to expand path; could not load sqlite database path '%s': %w", dbConfig.Path, err)
		}
	}

	_, err = os.Stat(path)
	if err != nil {
		// Attempt to create file
		file, err := os.Create(path)
		if err != nil {
			return nil, fmt.Errorf("could not load sqlite database path '%s'; failed to create: %w", dbConfig.Path, err)
		}
		defer file.Close()
	}

	db, err := sql.Open("sqlite3", dbConfig.GetDsn())
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database '%s': %w", dbConfig.GetDsn(), err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database '%s': %w", dbConfig.GetDsn(), err)
	}

	return &service{
		cfg:             dbConfig,
		sq:              sq.StatementBuilder.PlaceholderFormat(dbConfig.GetPlaceholderFormat()),
		db:              db,
		cursorEncryptor: pagination.NewRandomCursorEncryptor(),
		logger:          l,
	}, nil
}

// NewPostgresConnection creates a new database connection to a Postgres database.
func NewPostgresConnection(dbConfig *config.DatabasePostgres, l *slog.Logger) (DB, error) {
	db, err := sql.Open("pgx", dbConfig.GetDsn())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database '%s': %w", dbConfig.GetDsn(), err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres database '%s': %w", dbConfig.GetDsn(), err)
	}

	return &service{
		cfg:             dbConfig,
		sq:              sq.StatementBuilder.PlaceholderFormat(dbConfig.GetPlaceholderFormat()),
		db:              db,
		cursorEncryptor: pagination.NewRandomCursorEncryptor(),
		logger:          l,
	}, nil
}

type service struct {
	cfg             config.DatabaseImpl
	sq              sq.StatementBuilderType
	db              *sql.DB
	cursorEncryptor pagination.CursorEncryptor
	logger          *slog.Logger
}

func (s *service) SetCursorEncryptor(e pagination.CursorEncryptor) {
	s.cursorEncryptor = e
}

func (s *service) Ping(ctx context.Context) bool {
	if err := s.db.Ping(); err != nil {
		s.logger.Error("failed to ping database")
		return false
	}

	_, err := s.db.Exec("SELECT 1")
	if err != nil {
		s.logger.Error("failed to ping database with query")
		log.Println(fmt.Errorf("failed to connect to database: %w", err))
		return false
	}

	return true
}

var _ DB = (*service)(nil)
