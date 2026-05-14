package core

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/auth_methods/api_key"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
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

	o2FactoryOnce sync.Once
	o2Factory     oauth2.Factory

	apiKeyFactoryOnce sync.Once
	apiKeyFactory     api_key.Factory
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
	return s
}

var _ iface.C = (*service)(nil)
