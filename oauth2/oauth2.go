package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
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
	connection database.Connection
	connector  *config.Connector
	auth       *config.AuthOAuth2
	httpf      httpf.F
	encrypt    encrypt.E
	state      *state
	logger     *slog.Logger
}

func newOAuth2(
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
	logger *slog.Logger,
	connection database.Connection,
	connector config.Connector,
) *OAuth2 {
	auth, ok := connector.Auth.(*config.AuthOAuth2)
	if !ok {
		panic(fmt.Sprintf("connector id %s is not an oauth2 connector", connector.Id))
	}

	return &OAuth2{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		connection: connection,
		auth:       auth,
		httpf:      httpf,
		encrypt:    encrypt,
		connector:  &connector,
		logger:     logger,
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
		return errors.Wrapf(result.Err(), "failed to set state in redis for session status for connector %s", o.connector.Id)
	}

	return nil
}

func (o *OAuth2) CancelSessionAfterAuth() bool {
	return o.state.CancelSessionAfterAuth
}
