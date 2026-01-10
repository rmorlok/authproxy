package core

import (
	"context"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

const (
	ProbeOutcomeUnknown = "unknown"
	ProbeOutcomeSuccess = "success"
	ProbeOutcomeError   = "error"
)

type probeBase struct {
	cfg    *cschema.Probe
	s      *service
	cv     *ConnectorVersion
	c      *connection
	logger *slog.Logger
}

func (p *probeBase) IsPeriodic() bool {
	return p.cfg.Period != nil || p.cfg.Cron != nil
}

func (p *probeBase) GetId() string {
	return p.cfg.Id
}

func (p *probeBase) Logger() *slog.Logger {
	return p.logger
}

func (p *probeBase) GetScheduleString() string {
	if !p.IsPeriodic() {
		return ""
	}

	if p.cfg.Period != nil {
		return "@every " + p.cfg.Period.Duration.String()
	}

	return *p.cfg.Cron
}

func (p *probeBase) recordInvokeOutcome(
	ctx context.Context,
	invoke func(ctx context.Context) (string, error),
) (string, error) {
	clock := apctx.GetClock(ctx)
	start := clock.Now()
	outcome := ProbeOutcomeUnknown
	var err error

	p.logger.Debug("invoking probe")
	defer func() {
		duration := clock.Now().Sub(start)
		if err != nil {
			p.logger.Error("probe failed", "outcome", outcome, "duration", duration, "error", err)
		} else {
			p.logger.Debug("probe succeeded", "outcome", outcome, "duration", duration)
		}
	}()

	outcome, err = invoke(ctx)

	return outcome, err
}

func NewProbe(cfg *cschema.Probe, s *service, cv *ConnectorVersion, c *connection) iface.Probe {
	base := probeBase{
		cfg: cfg,
		s:   s,
		cv:  cv,
		c:   c,
		logger: aplog.NewBuilder(s.logger).
			With("probe_id", cfg.Id).
			WithNamespace(c.Namespace).
			WithConnectionId(c.Id).
			WithConnectorId(cv.Id).
			WithConnectorVersion(cv.Version).
			Build(),
	}

	if cfg.Http != nil || cfg.ProxyHttp != nil {
		// Raw HTTP probe
		return &probeHttp{base}
	} else {
		// This perhaps should be an error
		return &probeNoOp{base}
	}
}

var _ aplog.HasLogger = (*probeBase)(nil)
