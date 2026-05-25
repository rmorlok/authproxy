package core

import (
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	"github.com/rmorlok/authproxy/internal/auth_methods/api_key"
	"github.com/rmorlok/authproxy/internal/auth_methods/no_auth"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

type service struct {
	cfg     config.C
	db      database.DB
	encrypt encrypt.E
	r       apredis.Client
	httpf   httpf.F
	ac      apasynq.Client
	logger  *slog.Logger

	// rlCache is the enforcer-shared rate-limit cache. Optional —
	// DryRunRateLimit gracefully treats nil as "no rules to evaluate"
	// so test setups that don't exercise the dry-run path don't need
	// to wire one.
	rlCache ratelimit.Cache

	telProviders *aptelemetry.Providers
	telCfg       *sconfig.Telemetry

	// authMethodFactories is the uniform auth-method dispatch registry.
	// Populated once at NewCoreService and keyed by cschema.AuthType.
	// Resolved by getAuthMethodFactory(connector); call sites that need
	// per-method extras (e.g. oauth2.Factory.NewOAuth2) type-assert the
	// returned auth_methods.Factory at the use site, guarded by the same
	// auth-type check the caller already performs.
	authMethodFactories map[cschema.AuthType]auth_methods.Factory
}

// Option configures optional dependencies on the core service. The
// functional-options shape keeps NewCoreService non-breaking as we add
// wiring (rate-limit cache, telemetry hooks, etc.) — only callers that
// need the new dependency change.
type Option func(*service)

// WithRateLimitCache wires the in-memory rate-limit rule cache the
// enforcer reads. Required by C.DryRunRateLimit; harmless to omit when
// the dry-run path isn't exercised.
func WithRateLimitCache(c ratelimit.Cache) Option {
	return func(s *service) { s.rlCache = c }
}

// WithTelemetry attaches OTel providers + the telemetry config so subsystem
// factories owned by the core service (e.g. the OAuth2 factory) can
// instrument their lifecycle operations. nil providers / disabled signals
// degrade to no-op.
func WithTelemetry(providers *aptelemetry.Providers, cfg *sconfig.Telemetry) Option {
	return func(s *service) {
		s.telProviders = providers
		s.telCfg = cfg
	}
}

// NewCoreService creates a new core service
func NewCoreService(
	cfg config.C,
	db database.DB,
	encrypt encrypt.E,
	r apredis.Client,
	httpf httpf.F,
	ac apasynq.Client,
	logger *slog.Logger,
	opts ...Option,
) iface.C {
	s := &service{
		cfg:     cfg,
		db:      db,
		encrypt: encrypt,
		r:       r,
		httpf:   httpf,
		ac:      ac,
		logger:  logger,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.authMethodFactories = s.buildAuthMethodFactories()
	return s
}

// buildAuthMethodFactories constructs the per-auth-method factories with
// their dependencies and returns the registry. Each factory takes a
// different dependency set, so this can't be a generic loop — but the
// resulting map is uniform, and downstream code dispatches through it
// without knowing the construction shapes.
func (s *service) buildAuthMethodFactories() map[cschema.AuthType]auth_methods.Factory {
	var oauth2Opts []oauth2.FactoryOption
	if s.telProviders != nil {
		oauth2Opts = append(oauth2Opts, oauth2.WithTelemetry(s.telProviders, s.telCfg))
	}

	return map[cschema.AuthType]auth_methods.Factory{
		cschema.AuthTypeOAuth2: oauth2.NewFactory(s.cfg, s.db, s.r, s, s.httpf, s.encrypt, s.logger, oauth2Opts...),
		cschema.AuthTypeAPIKey: api_key.NewFactory(s.db, s.encrypt, s.httpf, s.logger),
		cschema.AuthTypeNoAuth: no_auth.NewFactory(),
	}
}

// getAuthMethodFactory returns the auth-method factory for the connector's
// auth type. Returns nil if no factory is registered (i.e. an auth type
// the core service was not wired for); callers fall back to their own
// not-implemented handling.
func (s *service) getAuthMethodFactory(connector *cschema.Connector) auth_methods.Factory {
	if connector == nil || connector.Auth == nil {
		return nil
	}
	return s.authMethodFactories[connector.Auth.GetType()]
}

var _ iface.C = (*service)(nil)
