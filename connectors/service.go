package connectors

import (
	"github.com/rmorlok/authproxy/apasynq"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"log/slog"
)

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	ac      apasynq.Client
	logger  *slog.Logger
}

// NewConnectorsService creates a new connectors service
func NewConnectorsService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	ac apasynq.Client,
	logger *slog.Logger,
) C {
	return &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		ac:      ac,
		logger:  logger,
	}
}

var _ C = (*service)(nil)
