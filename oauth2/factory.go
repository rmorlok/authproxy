package oauth2

import (
	"context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

type Factory interface {
	NewOAuth2(connection database.Connection, connector *connectors.ConnectorVersion) *OAuth2
	GetOAuth2State(ctx context.Context, actor database.Actor, stateId uuid.UUID) (*OAuth2, error)
}

type factory struct {
	cfg        config.C
	db         database.DB
	redis      redis.R
	connectors connectors.C
	httpf      httpf.F
	encrypt    encrypt.E
	logger     *slog.Logger
}

func NewFactory(cfg config.C, db database.DB, redis redis.R, c connectors.C, httpf httpf.F, encrypt encrypt.E, logger *slog.Logger) Factory {
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

func (f *factory) NewOAuth2(connection database.Connection, connector *connectors.ConnectorVersion) *OAuth2 {
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

func (f *factory) GetOAuth2State(ctx context.Context, actor database.Actor, stateId uuid.UUID) (*OAuth2, error) {
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
