package oauth2

import (
	"context"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/config"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type factory struct {
	cfg        config.C
	db         database.DB
	redis      apredis.Client
	connectors coreIface.C
	httpf      httpf.F
	encrypt    encrypt.E
	logger     *slog.Logger
	tel        *telemetry
}

// FactoryOption configures a Factory at construction time. The functional-
// options shape keeps NewFactory non-breaking for call sites that don't
// need telemetry instrumentation (the majority during incremental rollout).
type FactoryOption func(*factoryOptions)

type factoryOptions struct {
	providers *aptelemetry.Providers
	telCfg    *sconfig.Telemetry
}

// WithTelemetry registers the OTel providers + telemetry config so the
// constructed oAuth2Connection emits lifecycle spans + refresh / revocation
// / token-exchange counters. When providers is nil or in no-op mode (or
// both trace + metric signals are off), the resulting telemetry surface is
// inert and the factory behaves identically to a call with no options.
func WithTelemetry(providers *aptelemetry.Providers, telCfg *sconfig.Telemetry) FactoryOption {
	return func(o *factoryOptions) {
		o.providers = providers
		o.telCfg = telCfg
	}
}

func NewFactory(cfg config.C, db database.DB, r apredis.Client, c coreIface.C, httpf httpf.F, encrypt encrypt.E, logger *slog.Logger, opts ...FactoryOption) Factory {
	resolved := &factoryOptions{}
	for _, opt := range opts {
		opt(resolved)
	}

	tel, err := newTelemetry(resolved.providers, resolved.telCfg)
	if err != nil {
		// Telemetry construction failure is a programmer error (bad meter
		// name, etc.). Loud-log and continue with an inert telemetry —
		// failing the whole factory over instrumentation plumbing would be
		// the wrong trade-off.
		logger.Error("oauth2: failed to construct telemetry", "error", err)
		tel = &telemetry{}
	}

	return &factory{
		cfg:        cfg,
		db:         db,
		redis:      r,
		connectors: c,
		httpf:      httpf,
		encrypt:    encrypt,
		logger:     logger,
		tel:        tel,
	}
}

func (f *factory) NewOAuth2(connection coreIface.Connection) OAuth2Connection {
	return f.newConnection(connection)
}

// NewAuthenticator returns the same oAuth2Connection instance typed as an
// auth_methods.Authenticator — the per-connection state is identical, only
// the surfaced interface differs.
func (f *factory) NewAuthenticator(connection coreIface.Connection) auth_methods.Authenticator {
	return f.newConnection(connection)
}

func (f *factory) newConnection(connection coreIface.Connection) *oAuth2Connection {
	conn := newOAuth2(
		f.cfg,
		f.db,
		f.redis,
		f.connectors,
		f.encrypt,
		f.logger,
		f.httpf,
		connection,
	)
	conn.tel = f.tel
	return conn
}

var _ auth_methods.Factory = (*factory)(nil)

func (f *factory) GetOAuth2State(ctx context.Context, actor IActorData, stateId apid.ID) (OAuth2Connection, error) {
	conn, err := getOAuth2State(
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
	if err != nil {
		return nil, err
	}
	if oc, ok := conn.(*oAuth2Connection); ok {
		oc.tel = f.tel
	}
	return conn, nil
}
