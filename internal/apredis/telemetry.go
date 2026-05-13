package apredis

import (
	"fmt"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryOpts holds the resolved OTel providers + config used when
// instrumenting a redis client. nil providers, providers in no-op mode, or
// both signals disabled degrade to a plain client with no hooks attached.
type telemetryOpts struct {
	providers *aptelemetry.Providers
	cfg       *sconfig.Telemetry
}

// Option configures redis client construction. WithTelemetry is currently
// the only option; the functional-options shape keeps the constructor
// signatures non-breaking for existing callers (e.g. test_config.go) that
// don't need telemetry.
type Option func(*telemetryOpts)

// WithTelemetry causes the constructed redis client to be instrumented with
// OTel spans + metrics via the redis/go-redis redisotel contrib. When
// providers is nil or in no-op mode, or when both trace and metric signals
// are off in cfg, this is silently inert and a plain client is returned.
func WithTelemetry(providers *aptelemetry.Providers, cfg *sconfig.Telemetry) Option {
	return func(o *telemetryOpts) {
		o.providers = providers
		o.cfg = cfg
	}
}

// resolveOpts collapses zero or more Options into the effective settings.
func resolveOpts(opts []Option) *telemetryOpts {
	o := &telemetryOpts{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// telemetryEnabled reports whether the resolved telemetry options should
// instrument the redis client. Both a live providers handle and at least one
// enabled signal are required.
func (o *telemetryOpts) telemetryEnabled() bool {
	if o == nil || o.providers == nil || !o.providers.Enabled {
		return false
	}
	return o.cfg.TracesEnabled() || o.cfg.MetricsEnabled()
}

// instrumentClient attaches redisotel tracing and / or metric hooks to the
// supplied client based on the resolved telemetry options. Safe to call with
// disabled telemetry — the function returns early when no signal is enabled.
//
// Both hooks are best-effort: a failure to attach a hook is logged-but-
// nonfatal (returned as a wrapped error so the caller can decide). Failing
// the whole client construction over telemetry plumbing would be a worse
// outcome than missing instrumentation, so call sites should typically
// continue on error.
func instrumentClient(client *redis.Client, opts *telemetryOpts) error {
	if !opts.telemetryEnabled() {
		return nil
	}

	if opts.cfg.TracesEnabled() {
		if err := redisotel.InstrumentTracing(
			client,
			redisotel.WithTracerProvider(opts.providers.TracerProvider),
		); err != nil {
			return fmt.Errorf("apredis: instrument tracing: %w", err)
		}
	}

	if opts.cfg.MetricsEnabled() {
		if err := redisotel.InstrumentMetrics(
			client,
			redisotel.WithMeterProvider(opts.providers.MeterProvider),
		); err != nil {
			return fmt.Errorf("apredis: instrument metrics: %w", err)
		}
	}

	return nil
}
