package connectors

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/apasynq"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors/iface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
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
