package apasynq

import (
	"context"
	"errors"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// asynqTelemetryFixture wires an OTel SDK with an in-memory span recorder +
// a manual metric reader so the apasynq telemetry tests can introspect
// emitted telemetry without a live exporter.
type asynqTelemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newAsynqTelemetryFixture(t *testing.T) *asynqTelemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &asynqTelemetryFixture{
		providers: &aptelemetry.Providers{
			Enabled:        true,
			TracerProvider: tp,
			MeterProvider:  mp,
			Propagator:     propagation.TraceContext{},
		},
		spans:  spans,
		reader: reader,
	}
}

func (f *asynqTelemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

func enabledPtr(b bool) *bool { return &b }

// runHandlerThroughMiddleware executes a handler (success or failure) through
// the telemetry middleware to produce span + metric output. No real asynq
// server / queue is involved — the middleware sees only the *asynq.Task and
// context.Background, which is the same surface it sees in production.
func runHandlerThroughMiddleware(t *testing.T, tel *Telemetry, handler asynq.HandlerFunc) error {
	t.Helper()
	wrapped := tel.Middleware()(handler)
	return wrapped.ProcessTask(context.Background(), asynq.NewTask("authproxy:test_task", []byte(`{"k":"v"}`)))
}

func TestAsynqTelemetry_HandlerEmitsSpanAndMetric(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	tel, err := NewTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)}, nil)
	require.NoError(t, err)

	require.NoError(t, runHandlerThroughMiddleware(t, tel, func(_ context.Context, _ *asynq.Task) error {
		return nil
	}))

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	span := gotSpans[0]

	require.Equal(t, "asynq.task authproxy:test_task", span.Name())
	require.Equal(t, codes.Unset, span.Status().Code, "success path must leave the span status unset")

	rm := fx.readMetrics(t)
	requireMetricEmitted(t, rm, "authproxy.asynq.task.duration")
}

func TestAsynqTelemetry_HandlerErrorMarksSpanErrored(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	tel, err := NewTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)}, nil)
	require.NoError(t, err)

	wantErr := errors.New("handler blew up")
	err = runHandlerThroughMiddleware(t, tel, func(_ context.Context, _ *asynq.Task) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr, "the middleware must propagate the handler error")

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	span := gotSpans[0]

	require.Equal(t, codes.Error, span.Status().Code, "error path must mark the span errored")
	require.NotEmpty(t, span.Events(), "RecordError should add an exception event")
}

func TestAsynqTelemetry_NoOpWhenProvidersDisabled(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	tel, err := NewTelemetry(aptelemetry.NoopProviders(), &sconfig.Telemetry{Enabled: enabledPtr(true)}, nil)
	require.NoError(t, err)

	require.NoError(t, runHandlerThroughMiddleware(t, tel, func(_ context.Context, _ *asynq.Task) error {
		return nil
	}))

	require.Empty(t, fx.spans.Ended(), "no spans when providers are no-op")
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics, "no metrics when providers are no-op")
}

func TestAsynqTelemetry_NoOpWhenAllSignalsOff(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	off := false
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	}
	tel, err := NewTelemetry(fx.providers, cfg, nil)
	require.NoError(t, err)

	require.NoError(t, runHandlerThroughMiddleware(t, tel, func(_ context.Context, _ *asynq.Task) error {
		return nil
	}))

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestAsynqTelemetry_NilTelemetryIsSafe(t *testing.T) {
	// Defensive: a nil *Telemetry must produce a working identity
	// middleware and a no-op scheduler-sync wrapper. The scheduler
	// constructs an *apasynq.Telemetry unconditionally today, but
	// defensive nil handling avoids latent NPEs if call sites change.
	var tel *Telemetry

	wrapped := tel.Middleware()(asynq.HandlerFunc(func(_ context.Context, _ *asynq.Task) error {
		return nil
	}))
	require.NoError(t, wrapped.ProcessTask(context.Background(), asynq.NewTask("x", nil)))

	configs, err := tel.WithSchedulerSyncSpan(context.Background(), func() ([]*asynq.PeriodicTaskConfig, error) {
		return []*asynq.PeriodicTaskConfig{{Cronspec: "@hourly", Task: asynq.NewTask("x", nil)}}, nil
	})
	require.NoError(t, err)
	require.Len(t, configs, 1)
}

func TestAsynqTelemetry_SchedulerSyncSpan(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	tel, err := NewTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)}, nil)
	require.NoError(t, err)

	configs, err := tel.WithSchedulerSyncSpan(context.Background(), func() ([]*asynq.PeriodicTaskConfig, error) {
		return []*asynq.PeriodicTaskConfig{
			{Cronspec: "@hourly", Task: asynq.NewTask("a", nil)},
			{Cronspec: "@daily", Task: asynq.NewTask("b", nil)},
		}, nil
	})
	require.NoError(t, err)
	require.Len(t, configs, 2)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, "asynq.scheduler.sync", gotSpans[0].Name())
}

func TestAsynqTelemetry_SchedulerSyncErrorMarksSpan(t *testing.T) {
	fx := newAsynqTelemetryFixture(t)
	tel, err := NewTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)}, nil)
	require.NoError(t, err)

	wantErr := errors.New("registrar fetch failed")
	_, err = tel.WithSchedulerSyncSpan(context.Background(), func() ([]*asynq.PeriodicTaskConfig, error) {
		return nil, wantErr
	})
	require.ErrorIs(t, err, wantErr)

	gotSpans := fx.spans.Ended()
	require.Len(t, gotSpans, 1)
	require.Equal(t, codes.Error, gotSpans[0].Status().Code)
}

// --- helpers ----------------------------------------------------------------

func requireMetricEmitted(t *testing.T, rm metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return
			}
		}
	}
	t.Fatalf("metric %q was expected but not emitted", name)
}
