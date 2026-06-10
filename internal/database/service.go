package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// NewService creates the AuthProxy database service using an already-open and
// fully configured database/sql handle. The caller owns the SQL handle and is
// responsible for closing it.
func NewService(db *sql.DB, dbConfig config.DatabaseImpl, logger *slog.Logger) (DB, error) {
	if db == nil {
		return nil, fmt.Errorf("database handle is required")
	}
	if dbConfig == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database '%s': %w", dbConfig.GetDsn(), err)
	}

	return &service{
		cfg:             dbConfig,
		sq:              sq.StatementBuilder.PlaceholderFormat(dbConfig.GetPlaceholderFormat()),
		db:              db,
		cursorEncryptor: pagination.NewRandomCursorEncryptor(),
		logger:          logger,
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
