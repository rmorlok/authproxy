package core

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/ratelimit"
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

	o2FactoryOnce sync.Once
	o2Factory     oauth2.Factory
}

// Option configures optional dependencies on the core service. Functional
// options keep the NewCoreService signature stable as we add wiring (the
// rate-limit cache, telemetry hooks, etc.) — only callers that need the
// new dependency change.
type Option func(*service)

// WithRateLimitCache wires the in-memory rate-limit rule cache the
// enforcer reads. Required by C.DryRunRateLimit; harmless to omit when
// the dry-run path isn't exercised.
func WithRateLimitCache(c ratelimit.Cache) Option {
	return func(s *service) { s.rlCache = c }
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
