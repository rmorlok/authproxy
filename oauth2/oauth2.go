package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
)

type OAuth2 struct {
	cfg        config.C
	db         database.DB
	redis      redis.R
	connectors connectors.C
	encrypt    encrypt.E
	logger     *slog.Logger
	auth       *config.AuthOAuth2
	httpf      httpf.F

	connection database.Connection
	cv         *connectors.ConnectorVersion
	state      *state
}

func newOAuth2(
	cfg config.C,
	db database.DB,
	redis redis.R,
	c connectors.C,
	encrypt encrypt.E,
	logger *slog.Logger,
	httpf httpf.F,
	connection database.Connection,
	cv *connectors.ConnectorVersion,
) *OAuth2 {
	connector := cv.GetDefinition()
	auth, ok := connector.Auth.(*config.AuthOAuth2)
	if !ok {
		panic(fmt.Sprintf("connector id %s is not an oauth2 connector", connector.Id))
	}

	return &OAuth2{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		connectors: c,
		encrypt:    encrypt,
		logger:     logger,
		auth:       auth,

		connection: connection,
		httpf:      httpf,
		cv:         cv,
	}
}

func (o *OAuth2) RecordCancelSessionAfterAuth(ctx context.Context, shouldCancel bool) error {
	if shouldCancel == o.state.CancelSessionAfterAuth {
		return nil
	}

	o.state.CancelSessionAfterAuth = shouldCancel
	ttl := o.state.ExpiresAt.Sub(apctx.GetClock(ctx).Now())

	result := o.redis.Client().Set(ctx, getStateRedisKey(o.state.Id), o.state, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for session status for connector %s", o.cv.ID)
	}

	return nil
}

func (o *OAuth2) CancelSessionAfterAuth() bool {
	return o.state.CancelSessionAfterAuth
}
