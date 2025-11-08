package core

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	r       apredis.Client
	httpf   httpf.F
	ac      apasynq.Client
	logger  *slog.Logger

	o2FactoryOnce sync.Once
	o2Factory     oauth2.Factory
}

// NewConnectorsService creates a new connectors service
func NewConnectorsService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	r apredis.Client,
	httpf httpf.F,
	ac apasynq.Client,
	logger *slog.Logger,
) iface.C {
	return &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		r:       r,
		httpf:   httpf,
		ac:      ac,
		logger:  logger,
	}
}

var _ iface.C = (*service)(nil)
