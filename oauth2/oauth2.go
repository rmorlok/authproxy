package oauth2

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"log/slog"
)

type oAuth2Connection struct {
	cfg        config.C
	db         database.DB
	r          apredis.Client
	connectors connIface.C
	encrypt    encrypt.E
	logger     *slog.Logger
	auth       *config.AuthOAuth2
	httpf      httpf.F

	connection database.Connection
	cv         connIface.ConnectorVersion
	state      *state
}

var _ OAuth2Connection = (*oAuth2Connection)(nil)

func newOAuth2(
	cfg config.C,
	db database.DB,
	r apredis.Client,
	c connIface.C,
	encrypt encrypt.E,
	logger *slog.Logger,
	httpf httpf.F,
	connection database.Connection,
	cv connIface.ConnectorVersion,
) *oAuth2Connection {
	connector := cv.GetDefinition()
	auth, ok := connector.Auth.(*config.AuthOAuth2)
	if !ok {
		panic(fmt.Sprintf("connector id %s is not an oauth2 connector", connector.Id))
	}

	return &oAuth2Connection{
		cfg:        cfg,
		db:         db,
		r:          r,
		connectors: c,
		encrypt:    encrypt,
		logger:     logger,
		auth:       auth,

		connection: connection,
		httpf:      httpf,
		cv:         cv,
	}
}

func (o *oAuth2Connection) RecordCancelSessionAfterAuth(ctx context.Context, shouldCancel bool) error {
	if shouldCancel == o.state.CancelSessionAfterAuth {
		return nil
	}

	o.state.CancelSessionAfterAuth = shouldCancel
	ttl := o.state.ExpiresAt.Sub(apctx.GetClock(ctx).Now())

	result := o.r.Set(ctx, getStateRedisKey(o.state.Id), o.state, ttl)
	if result.Err() != nil {
		return errors.Wrapf(result.Err(), "failed to set state in redis for session status for connector %s", o.cv.GetID())
	}

	return nil
}

func (o *oAuth2Connection) CancelSessionAfterAuth() bool {
	return o.state.CancelSessionAfterAuth
}
