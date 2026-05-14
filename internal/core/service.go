package core

import (
	"log/slog"
	"sync"

	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/auth_methods/oauth2"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
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

	telProviders *aptelemetry.Providers
	telCfg       *sconfig.Telemetry

	o2FactoryOnce sync.Once
	o2Factory     oauth2.Factory
}

// Option configures a core service at construction time. The functional-
// options shape keeps NewCoreService non-breaking for existing call sites.
type Option func(*service)

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
