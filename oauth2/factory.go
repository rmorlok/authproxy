package oauth2

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

type factory struct {
	cfg        config.C
	db         database.DB
	redis      redis.R
	connectors connIface.C
	httpf      httpf.F
	encrypt    encrypt.E
	logger     *slog.Logger
}

func NewFactory(cfg config.C, db database.DB, redis redis.R, c connIface.C, httpf httpf.F, encrypt encrypt.E, logger *slog.Logger) Factory {
	return &factory{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		connectors: c,
		httpf:      httpf,
		encrypt:    encrypt,
		logger:     logger,
	}
}

func (f *factory) NewOAuth2(connection database.Connection, connector connIface.ConnectorVersion) OAuth2Connection {
	return newOAuth2(
		f.cfg,
		f.db,
		f.redis,
		f.connectors,
		f.encrypt,
		f.logger,
		f.httpf,
		connection,
		connector,
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
