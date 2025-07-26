package connectors

import (
	"github.com/rmorlok/authproxy/apasynq"
	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	redis   redis.R
	httpf   httpf.F
	ac      apasynq.Client
	logger  *slog.Logger
}

// NewConnectorsService creates a new connectors service
func NewConnectorsService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	redis redis.R,
	httpf httpf.F,
	ac apasynq.Client,
	logger *slog.Logger,
) connIface.C {
	return &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		redis:   redis,
		httpf:   httpf,
		ac:      ac,
		logger:  logger,
	}
}

var _ connIface.C = (*service)(nil)
