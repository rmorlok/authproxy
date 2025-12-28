package oauth2

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type oAuth2Connection struct {
	cfg        config.C
	db         database.DB
	r          apredis.Client
	connectors coreIface.C
	encrypt    encrypt.E
	logger     *slog.Logger
	auth       *config.AuthOAuth2
	httpf      httpf.F

	connection coreIface.Connection
	state      *state
}

var _ OAuth2Connection = (*oAuth2Connection)(nil)

func newOAuth2(
	cfg config.C,
	db database.DB,
	r apredis.Client,
	c coreIface.C,
	encrypt encrypt.E,
	logger *slog.Logger,
	httpf httpf.F,
	connection coreIface.Connection,
) *oAuth2Connection {
	cv := connection.GetConnectorVersionEntity()
	connector := cv.GetDefinition()
	auth, ok := connector.Auth.Inner().(*config.AuthOAuth2)
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
		return errors.Wrapf(result.Err(), "failed to set state in redis for session status for connection %s", o.connection.GetId())
	}

	return nil
}

func (o *oAuth2Connection) CancelSessionAfterAuth() bool {
	return o.state.CancelSessionAfterAuth
}
