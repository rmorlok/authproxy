package oauth2

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type factory struct {
	cfg        config.C
	db         database.DB
	redis      apredis.Client
	connectors coreIface.C
	httpf      httpf.F
	encrypt    encrypt.E
	logger     *slog.Logger
}

func NewFactory(cfg config.C, db database.DB, r apredis.Client, c coreIface.C, httpf httpf.F, encrypt encrypt.E, logger *slog.Logger) Factory {
	return &factory{
		cfg:        cfg,
		db:         db,
		redis:      r,
		connectors: c,
		httpf:      httpf,
		encrypt:    encrypt,
		logger:     logger,
	}
}

func (f *factory) NewOAuth2(connection coreIface.Connection) OAuth2Connection {
	return newOAuth2(
		f.cfg,
		f.db,
		f.redis,
		f.connectors,
		f.encrypt,
		f.logger,
		f.httpf,
		connection,
	)
}

func (f *factory) GetOAuth2State(ctx context.Context, actor database.Actor, stateId uuid.UUID) (OAuth2Connection, error) {
	return getOAuth2State(
		ctx,
		f.cfg,
		f.db,
		f.redis,
		f.connectors,
		f.httpf,
		f.encrypt,
		f.logger,
		actor,
		stateId,
	)
}
