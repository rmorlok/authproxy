package apredis

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// redisTelemetryFixture wires an OTel SDK with an in-memory span recorder +
// a manual metric reader so the apredis telemetry tests can introspect
// emitted telemetry without a live exporter.
type redisTelemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newRedisTelemetryFixture(t *testing.T) *redisTelemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &redisTelemetryFixture{
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

func (f *redisTelemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

// newIsolatedRedisClient brings up a private miniredis instance per test
// (separate from the package-level singleton used by NewMiniredis) so that
// instrumentation hooks can be attached fresh per test without fighting
// sync.Once gating on the shared singleton.
func newIsolatedRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := redis.NewClient(&redis.Options{
		Addr:     srv.Addr(),
		Protocol: 2,
	})
	t.Cleanup(func() { _ = client.Close() })

	require.NoError(t, client.Ping(context.Background()).Err())
	return client
}

func enabledPtr(b bool) *bool { return &b }

func TestRedisTelemetry_CommandEmitsSpanAndMetric(t *testing.T) {
	fx := newRedisTelemetryFixture(t)
	tel := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	client := newIsolatedRedisClient(t)
	require.NoError(t, instrumentClient(client, resolveOpts([]Option{WithTelemetry(fx.providers, tel)})))

	// SET / GET drives at least one command through the instrumented client.
	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())
	val, err := client.Get(context.Background(), "k").Result()
	require.NoError(t, err)
	require.Equal(t, "v", val)

	require.NotEmpty(t, fx.spans.Ended(), "instrumented redis commands must emit spans")
	rm := fx.readMetrics(t)
	require.NotEmpty(t, rm.ScopeMetrics, "instrumented redis commands must emit metrics")
}

func TestRedisTelemetry_NoOpWhenProvidersDisabled(t *testing.T) {
	fx := newRedisTelemetryFixture(t)
	tel := &sconfig.Telemetry{Enabled: enabledPtr(true)} // signals would be on, but providers say disabled

	client := newIsolatedRedisClient(t)
	// No-op providers — instrumentClient must skip hook attachment.
	require.NoError(t, instrumentClient(client, resolveOpts([]Option{WithTelemetry(aptelemetry.NoopProviders(), tel)})))

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())

	require.Empty(t, fx.spans.Ended(), "no spans when telemetry providers are no-op")
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics, "no metrics when telemetry providers are no-op")
}

func TestRedisTelemetry_NoOpWhenAllSignalsOff(t *testing.T) {
	fx := newRedisTelemetryFixture(t)
	off := false
	on := true
	tel := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	}

	client := newIsolatedRedisClient(t)
	require.NoError(t, instrumentClient(client, resolveOpts([]Option{WithTelemetry(fx.providers, tel)})))

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestRedisTelemetry_NoOpWhenNoOptions(t *testing.T) {
	// No WithTelemetry option supplied at all — exercises the "default
	// path" callers like test_config.go take.
	fx := newRedisTelemetryFixture(t)

	client := newIsolatedRedisClient(t)
	require.NoError(t, instrumentClient(client, resolveOpts(nil)))

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestRedisTelemetry_TracesOnlyDoesNotEmitMetrics(t *testing.T) {
	// Signals are independent: turning metrics off must not silently emit
	// metrics, and vice versa.
	fx := newRedisTelemetryFixture(t)
	on := true
	off := false
	tel := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &on, Metrics: &off},
	}

	client := newIsolatedRedisClient(t)
	require.NoError(t, instrumentClient(client, resolveOpts([]Option{WithTelemetry(fx.providers, tel)})))

	require.NoError(t, client.Set(context.Background(), "k", "v", 0).Err())

	require.NotEmpty(t, fx.spans.Ended(), "tracing must be active when traces signal is on")
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics, "metrics must remain silent when the metrics signal is off")
}
