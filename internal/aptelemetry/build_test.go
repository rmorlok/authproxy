package aptelemetry

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

func TestNew_NilConfig_ReturnsNoopProviders(t *testing.T) {
	p, err := New(context.Background(), "api", "", nil)
	require.NoError(t, err)
	require.NotNil(t, p)
	require.False(t, p.Enabled)

	requireNoopTracerProvider(t, p.TracerProvider)
	requireNoopMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider)

	// Propagator is always populated so call sites can extract context.
	require.NotNil(t, p.Propagator)

	// Shutdown is a no-op.
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestNew_DisabledConfig_ReturnsNoopProviders(t *testing.T) {
	off := false
	cfg := &sconfig.Telemetry{Enabled: &off}

	p, err := New(context.Background(), "api", "", cfg)
	require.NoError(t, err)
	require.False(t, p.Enabled)
	requireNoopTracerProvider(t, p.TracerProvider)
	requireNoopMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider)
}

func TestNew_EnabledWithOtlpEndpoint_InitializesAndShutsDown(t *testing.T) {
	// Reserve a TCP port for an OTLP endpoint so exporter creation succeeds
	// without dialling anything real. Exporters lazily connect on first
	// export, so simply pointing at this address is enough for the bootstrap
	// path; we never actually export in this test.
	endpoint := reserveLocalTCPAddress(t)

	on := true
	off := false
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Exporter: &sconfig.TelemetryExporter{
			Endpoint: directStringValue("http://" + endpoint),
			Insecure: &on,
		},
		// Disable logs in this test — bootstrap path is identical for all
		// signals and turning logs off keeps the test goroutine count low.
		Signals: &sconfig.TelemetrySignals{Traces: &on, Metrics: &on, Logs: &off},
	}

	p, err := New(context.Background(), "api", "test-version", cfg)
	require.NoError(t, err)
	require.True(t, p.Enabled)

	requireRealTracerProvider(t, p.TracerProvider)
	requireRealMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider) // logs disabled

	// Shutdown contract: return within the configured timeout. With no
	// collector reachable, exporters legitimately report transport errors
	// during their final flush attempt; that's SDK behaviour, not a
	// bootstrap concern.
	requireShutdownWithinTimeout(t, p, 3*time.Second)

	// Idempotent — second call returns immediately and is a no-op.
	requireShutdownWithinTimeout(t, p, time.Second)
}

func TestNew_SignalsToggleIndependently(t *testing.T) {
	endpoint := reserveLocalTCPAddress(t)

	on := true
	off := false
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Exporter: &sconfig.TelemetryExporter{
			Endpoint: directStringValue("http://" + endpoint),
			Insecure: &on,
		},
		Signals: &sconfig.TelemetrySignals{Traces: &on, Metrics: &off, Logs: &off},
	}

	p, err := New(context.Background(), "api", "", cfg)
	require.NoError(t, err)

	requireRealTracerProvider(t, p.TracerProvider)
	requireNoopMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider)

	requireShutdownWithinTimeout(t, p, 3*time.Second)
}

// TestNew_EnabledButEmptyEndpointFallsThroughToNoop pins the soft-disable
// gate used by dev_config/default.yaml: telemetry.enabled may be true and
// the block fully populated, but with the exporter.endpoint env-var
// resolving to an empty string (default ""), New returns no-op providers
// rather than dialling localhost:4317. This is what gives the
// "observability profile not running -> no SDK warnings" UX dev expects.
func TestNew_EnabledButEmptyEndpointFallsThroughToNoop(t *testing.T) {
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Exporter: &sconfig.TelemetryExporter{
			// directStringValue("") gives a present-but-empty endpoint
			// — same shape as an env-var with an empty default that
			// the operator hasn't overridden.
			Endpoint: directStringValue(""),
		},
	}

	p, err := New(context.Background(), "api", "", cfg)
	require.NoError(t, err)
	require.False(t, p.Enabled, "empty endpoint must fall through to no-op providers")
	requireNoopTracerProvider(t, p.TracerProvider)
	requireNoopMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider)
}

func TestNoopProviders(t *testing.T) {
	p := NoopProviders()
	require.False(t, p.Enabled)
	requireNoopTracerProvider(t, p.TracerProvider)
	requireNoopMeterProvider(t, p.MeterProvider)
	requireNoopLoggerProvider(t, p.LoggerProvider)
	require.NotNil(t, p.Propagator)
	require.NoError(t, p.Shutdown(context.Background()))
}

// --- helpers -----------------------------------------------------------------

// directStringValue wraps a literal string in a StringValue using the direct
// variant, suitable for tests that want to bypass YAML parsing.
func directStringValue(v string) *sconfig.StringValue {
	return &sconfig.StringValue{InnerVal: &sconfig.StringValueDirect{Value: v, IsDirect: true}}
}

// requireShutdownWithinTimeout calls Shutdown with the given deadline and
// asserts the call returns before the deadline expires. The shutdown error
// itself is tolerated: with no real collector reachable, exporters report
// transport errors on their final flush attempt — that's expected SDK
// behaviour, distinct from "shutdown returned in time".
func requireShutdownWithinTimeout(t *testing.T, p *Providers, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = p.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout + time.Second):
		t.Fatalf("Providers.Shutdown did not return within %s", timeout+time.Second)
	}
}

// reserveLocalTCPAddress binds a port on 127.0.0.1 and closes the listener so
// the port is free again. The returned "host:port" is suitable for use as an
// OTLP endpoint that exporter clients can be configured against without
// anything actually listening — exporters connect lazily on first export.
func reserveLocalTCPAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

func requireNoopTracerProvider(t *testing.T, tp trace.TracerProvider) {
	t.Helper()
	got := fmt.Sprintf("%T", tp.Tracer("test"))
	expected := fmt.Sprintf("%T", tracenoop.NewTracerProvider().Tracer("test"))
	require.Equal(t, expected, got, "expected no-op tracer provider, got %T", tp)
}

func requireRealTracerProvider(t *testing.T, tp trace.TracerProvider) {
	t.Helper()
	got := fmt.Sprintf("%T", tp.Tracer("test"))
	noop := fmt.Sprintf("%T", tracenoop.NewTracerProvider().Tracer("test"))
	require.NotEqual(t, noop, got, "expected real tracer provider, got no-op (%T)", tp)
}

func requireNoopMeterProvider(t *testing.T, mp metric.MeterProvider) {
	t.Helper()
	got := fmt.Sprintf("%T", mp.Meter("test"))
	expected := fmt.Sprintf("%T", metricnoop.NewMeterProvider().Meter("test"))
	require.Equal(t, expected, got, "expected no-op meter provider, got %T", mp)
}

func requireRealMeterProvider(t *testing.T, mp metric.MeterProvider) {
	t.Helper()
	got := fmt.Sprintf("%T", mp.Meter("test"))
	noop := fmt.Sprintf("%T", metricnoop.NewMeterProvider().Meter("test"))
	require.NotEqual(t, noop, got, "expected real meter provider, got no-op (%T)", mp)
}

func requireNoopLoggerProvider(t *testing.T, lp log.LoggerProvider) {
	t.Helper()
	got := fmt.Sprintf("%T", lp.Logger("test"))
	expected := fmt.Sprintf("%T", lognoop.NewLoggerProvider().Logger("test"))
	require.Equal(t, expected, got, "expected no-op logger provider, got %T", lp)
}
