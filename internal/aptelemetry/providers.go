package aptelemetry

import (
	"context"
	"errors"
	"sync"

	"go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Providers bundles the OTel providers a service needs. When telemetry is
// disabled, the embedded providers are no-op implementations and Shutdown is
// a no-op.
type Providers struct {
	// Enabled reports whether the SDK was initialised. When false the
	// providers below are no-op.
	Enabled bool

	// TracerProvider produces tracers for span emission.
	TracerProvider trace.TracerProvider

	// MeterProvider produces meters for metric emission.
	MeterProvider metric.MeterProvider

	// LoggerProvider produces loggers for the OTel logs SDK. When the logs
	// signal is disabled (or telemetry is disabled), this is a no-op.
	LoggerProvider log.LoggerProvider

	// Propagator is the W3C TraceContext + Baggage composite propagator.
	// Always returned (even when disabled) so call sites can extract
	// inbound context unconditionally.
	Propagator propagation.TextMapPropagator

	shutdownMu sync.Mutex
	shutdowns  []func(context.Context) error
	shutDown   bool
}

// NoopProviders returns a Providers with no-op implementations and no
// shutdown work. Callers can use this when the application does not configure
// telemetry, when running in tests, or as a defensive default.
func NoopProviders() *Providers {
	return &Providers{
		Enabled:        false,
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  metricnoop.NewMeterProvider(),
		LoggerProvider: lognoop.NewLoggerProvider(),
		Propagator:     defaultPropagator(),
	}
}

// Shutdown flushes and shuts down every registered exporter / provider. Safe
// to call more than once; subsequent calls are no-ops. Errors from individual
// shutdown hooks are joined and returned together so a single failure does
// not mask the rest.
func (p *Providers) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	p.shutdownMu.Lock()
	if p.shutDown {
		p.shutdownMu.Unlock()
		return nil
	}
	p.shutDown = true
	hooks := p.shutdowns
	p.shutdowns = nil
	p.shutdownMu.Unlock()

	var errs []error
	for _, h := range hooks {
		if err := h(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// addShutdown registers a shutdown hook to be invoked by Shutdown. Hooks run
// in registration order.
func (p *Providers) addShutdown(fn func(context.Context) error) {
	if fn == nil {
		return
	}
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()
	p.shutdowns = append(p.shutdowns, fn)
}

// defaultPropagator returns the W3C TraceContext + Baggage composite
// propagator used by the SDK by default.
func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}
