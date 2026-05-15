package aplog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// inMemoryLogExporter buffers every Record passed to Export so tests can
// introspect what reaches the OTel logs pipeline. The sdk/log package
// doesn't ship a tracetest-style recorder for logs in v0.16, so we provide
// our own minimal exporter here.
type inMemoryLogExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *inMemoryLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}

func (e *inMemoryLogExporter) Shutdown(_ context.Context) error  { return nil }
func (e *inMemoryLogExporter) ForceFlush(_ context.Context) error { return nil }

func (e *inMemoryLogExporter) Records() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]sdklog.Record, len(e.records))
	copy(out, e.records)
	return out
}

// telemetryProviders builds enabled OTel providers with an in-memory log
// exporter (so we can assert what flows through the bridge). Traces and
// metrics aren't asserted in this test file; we instantiate real providers
// so the otelslog bridge's logger lookup succeeds.
func telemetryProviders(t *testing.T) (*aptelemetry.Providers, *inMemoryLogExporter) {
	t.Helper()

	exporter := &inMemoryLogExporter{}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	tp := sdktrace.NewTracerProvider()
	mp := sdkmetric.NewMeterProvider()

	t.Cleanup(func() {
		_ = lp.Shutdown(context.Background())
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &aptelemetry.Providers{
		Enabled:        true,
		TracerProvider: tp,
		MeterProvider:  mp,
		LoggerProvider: lp,
		Propagator:     propagation.TraceContext{},
	}, exporter
}

func enabledPtr(b bool) *bool { return &b }

// newJSONLogger wraps a buffer-backed slog.JSONHandler so tests can read
// the formatted bytes and assert attribute presence.
func newJSONLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// parseLastRecord parses the most recent JSON object emitted to buf.
func parseLastRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.NotEmpty(t, lines)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &record))
	return record
}

func TestWrapWithTelemetry_AddsTraceIDsWhenInTracedContext(t *testing.T) {
	providers, _ := telemetryProviders(t)

	base, buf := newJSONLogger()
	logger := WrapWithTelemetry(base, providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NotNil(t, logger)

	// Construct a context with a recording span — the wrapper must lift
	// trace_id + span_id onto every record emitted within it.
	tracer := providers.TracerProvider.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	logger.InfoContext(ctx, "hello")

	record := parseLastRecord(t, buf)
	require.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])
}

func TestWrapWithTelemetry_NoTraceAttrsOutsideTrace(t *testing.T) {
	providers, _ := telemetryProviders(t)

	base, buf := newJSONLogger()
	logger := WrapWithTelemetry(base, providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})

	logger.Info("hello") // no context, no span

	record := parseLastRecord(t, buf)
	_, hasTrace := record["trace_id"]
	_, hasSpan := record["span_id"]
	require.False(t, hasTrace, "trace_id must not be set when no span is in flight")
	require.False(t, hasSpan, "span_id must not be set when no span is in flight")
}

func TestWrapWithTelemetry_LogsSignalOnFansToOTelExporter(t *testing.T) {
	providers, exporter := telemetryProviders(t)

	base, _ := newJSONLogger()
	logger := WrapWithTelemetry(base, providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})

	logger.Info("hello via otel bridge")

	// Force-flush by shutting the LoggerProvider — the simple processor
	// would already have exported, but be defensive about it.
	require.NoError(t, providers.LoggerProvider.(*sdklog.LoggerProvider).ForceFlush(context.Background()))

	records := exporter.Records()
	require.NotEmpty(t, records, "logs signal on must fan records to the OTel exporter")
}

func TestWrapWithTelemetry_LogsSignalOffSkipsOTelFanOut(t *testing.T) {
	providers, exporter := telemetryProviders(t)

	off := false
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &on, Metrics: &on, Logs: &off},
	}

	base, buf := newJSONLogger()
	logger := WrapWithTelemetry(base, providers, cfg)

	logger.Info("hello via stderr only")

	require.NoError(t, providers.LoggerProvider.(*sdklog.LoggerProvider).ForceFlush(context.Background()))

	// Original sink still received the record.
	require.Contains(t, buf.String(), "hello via stderr only")
	// OTel exporter received nothing — the logs signal is off.
	require.Empty(t, exporter.Records(),
		"logs signal off must NOT fan records to the OTel exporter")
}

func TestWrapWithTelemetry_NoOpProvidersStillStampsTraceAttrs(t *testing.T) {
	// Even with no-op providers (telemetry block absent), the wrapper
	// must still stamp trace_id / span_id when in a traced context.
	// Operators who haven't enabled OTel still benefit from trace
	// correlation in logs as soon as someone starts a span in their app.
	base, buf := newJSONLogger()
	logger := WrapWithTelemetry(base, aptelemetry.NoopProviders(), nil)
	require.NotNil(t, logger)

	// Spin a real tracer so SpanContextFromContext returns a valid id.
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	logger.InfoContext(ctx, "hello")

	record := parseLastRecord(t, buf)
	require.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])
}

func TestWrapWithTelemetry_NilLoggerReturnedUnchanged(t *testing.T) {
	require.Nil(t, WrapWithTelemetry(nil, nil, nil))
}

func TestWrapWithTelemetry_PreservesWithAttrs(t *testing.T) {
	// The wrapped logger must still honor logger.With(...) for both
	// telemetry-on and telemetry-off configurations — losing inherited
	// attrs would break every existing log-builder helper.
	providers, _ := telemetryProviders(t)

	base, buf := newJSONLogger()
	wrapped := WrapWithTelemetry(base, providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})

	scoped := wrapped.With("service", "test", "component", "x")
	scoped.Info("hello")

	record := parseLastRecord(t, buf)
	require.Equal(t, "test", record["service"])
	require.Equal(t, "x", record["component"])
}

// TestWrapWithTelemetry_CorrelationIdStaysIndependent pins the spec line:
// "The existing CorrelationId field stays independent from trace_id —
// logs emitted within a request carry both." The trace-context wrapper
// stamps trace_id / span_id without touching any application-supplied
// attributes — including correlation_id added via slog.With.
func TestWrapWithTelemetry_CorrelationIdStaysIndependent(t *testing.T) {
	providers, _ := telemetryProviders(t)

	base, buf := newJSONLogger()
	logger := WrapWithTelemetry(base, providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})

	tracer := providers.TracerProvider.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	// Application attaches its correlation_id via slog.With as usual.
	scoped := logger.With("correlation_id", "corr-12345")
	scoped.InfoContext(ctx, "hello")

	record := parseLastRecord(t, buf)
	require.Equal(t, "corr-12345", record["correlation_id"],
		"correlation_id must pass through untouched by the trace-context handler")
	require.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"],
		"trace_id stamped alongside, not in place of, correlation_id")
}

func TestTraceContextHandler_DelegatesEnabled(t *testing.T) {
	// Inner handler is the source of truth for level decisions — the
	// wrapper must not raise or lower the effective threshold.
	base, _ := newJSONLogger() // JSON handler defaults to LevelDebug above
	h := &traceContextHandler{inner: base.Handler()}
	require.True(t, h.Enabled(context.Background(), slog.LevelDebug))
	require.True(t, h.Enabled(context.Background(), slog.LevelInfo))
	require.True(t, h.Enabled(context.Background(), slog.LevelError))
}

func TestMultiHandler_FanOutContinuesOnError(t *testing.T) {
	// A failure in one branch must not short-circuit the others.
	failing := &failingHandler{}
	base, buf := newJSONLogger()

	mh := &multiHandler{handlers: []slog.Handler{failing, base.Handler()}}
	logger := slog.New(mh)
	logger.Info("hello")

	require.Contains(t, buf.String(), "hello",
		"working handler must receive the record even when a sibling errors")
}

// failingHandler is a slog.Handler that always reports an error from
// Handle. Used to verify multiHandler's fan-out is fault-tolerant.
type failingHandler struct{}

func (failingHandler) Enabled(context.Context, slog.Level) bool      { return true }
func (failingHandler) Handle(context.Context, slog.Record) error     { return errSomething }
func (failingHandler) WithAttrs([]slog.Attr) slog.Handler            { return failingHandler{} }
func (failingHandler) WithGroup(string) slog.Handler                 { return failingHandler{} }

var errSomething = errSentinel("intentional test failure")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// silence trace dependency in the small smoke test
var _ trace.SpanContext = trace.SpanContext{}
