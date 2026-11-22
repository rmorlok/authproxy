package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// authoredLogExporter captures detached SDK records without contacting a
// collector. Its mutex also permits the SDK to flush or export concurrently.
type authoredLogExporter struct {
	mu      sync.Mutex      // Protects retained records during SDK callbacks.
	records []sdklog.Record // Clones owned by the test after Export returns.
}

// Export retains clones so later SDK reuse cannot change the observed records.
func (e *authoredLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

// Shutdown needs no cleanup because the exporter has no external resources.
func (*authoredLogExporter) Shutdown(context.Context) error { return nil }

// ForceFlush needs no work because Export stores records synchronously.
func (*authoredLogExporter) ForceFlush(context.Context) error { return nil }

// snapshot returns records independently of future SDK callbacks.
func (e *authoredLogExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]sdklog.Record, len(e.records))
	for i, record := range e.records {
		result[i] = record.Clone()
	}
	return result
}

// authoredLogMap requires a structured OTel map rather than serialized JSON
// text or a byte slice, then indexes its fields for attribution assertions.
func authoredLogMap(t *testing.T, value otellog.Value) map[string]otellog.Value {
	t.Helper()
	require.Equal(t, otellog.KindMap, value.Kind())
	fields := make(map[string]otellog.Value)
	for _, field := range value.AsMap() {
		fields[field.Key] = field.Value
	}
	return fields
}

// authoredLogNumber accepts either native numeric representation, rejecting
// strings and other lossy encodings while comparing the authored JSON value.
func authoredLogNumber(t *testing.T, value otellog.Value) float64 {
	t.Helper()
	switch value.Kind() {
	case otellog.KindInt64:
		return float64(value.AsInt64())
	case otellog.KindFloat64:
		return value.AsFloat64()
	default:
		t.Fatalf("expected numeric log field, got %v", value.Kind())
		return 0
	}
}

// TestAuthoredLoggingUsesConfiguredTelemetry preserves host and trace
// attribution while carrying detached authored fields through the existing
// JSON and OTel logging handlers with their structured types intact.
func TestAuthoredLoggingUsesConfiguredTelemetry(t *testing.T) {
	// Existing application telemetry owns both providers and the configured logger.
	exporter := &authoredLogExporter{}
	logs := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	traces := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		require.NoError(t, logs.Shutdown(context.Background()))
		require.NoError(t, traces.Shutdown(context.Background()))
	})
	var output bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&output, nil))
	enabled := true
	logger := aplog.WrapWithTelemetry(base, &aptelemetry.Providers{
		Enabled: true, LoggerProvider: logs, TracerProvider: traces,
	}, &sconfig.Telemetry{Enabled: &enabled}).With(
		"connection_id", "cxn_captured", "tool_id", "tol_captured", "revision", 7,
	)
	ctx, span := traces.Tracer("authored-logging-test").Start(context.Background(), "invocation")
	defer span.End()
	program := compileAsync(t, `async function execute() {
		console.info("page loaded", {
			count: 2, nested: {enabled: true}, ids: [1, 2],
			trace_id: "authored", connection_id: "cxn_authored"
		});
		return true;
	}`, nil)
	result, err := program.Evaluate(ctx, func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
		invocation.Logger = logger
		return nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(result))
	require.NoError(t, logs.ForceFlush(context.Background()))

	// The normal JSON sink keeps fixed host fields and context-derived trace IDs.
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "JavaScript authored log", record["msg"])
	require.Equal(t, "cxn_captured", record["connection_id"])
	require.Equal(t, "tol_captured", record["tool_id"])
	require.Equal(t, float64(7), record["revision"])
	require.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])
	require.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])

	// OTel receives the same attribution and typed nested fields, never raw JSON bytes.
	records := exporter.snapshot()
	require.Len(t, records, 1)
	exported := records[0]
	require.Equal(t, "JavaScript authored log", exported.Body().AsString())
	require.Equal(t, span.SpanContext().TraceID(), exported.TraceID())
	require.Equal(t, span.SpanContext().SpanID(), exported.SpanID())
	attributes := make(map[string]otellog.Value)
	exported.WalkAttributes(func(attribute otellog.KeyValue) bool {
		attributes[attribute.Key] = attribute.Value
		return true
	})
	require.Equal(t, "cxn_captured", attributes["connection_id"].AsString())
	require.Equal(t, "tol_captured", attributes["tool_id"].AsString())
	require.Equal(t, float64(7), authoredLogNumber(t, attributes["revision"]))
	authored := authoredLogMap(t, attributes["javascript"])
	require.Equal(t, "page loaded", authored["message"].AsString())
	fields := authoredLogMap(t, authored["fields"])
	require.Equal(t, float64(2), authoredLogNumber(t, fields["count"]))
	require.Equal(t, "authored", fields["trace_id"].AsString())
	require.Equal(t, "cxn_authored", fields["connection_id"].AsString())
	nested := authoredLogMap(t, fields["nested"])
	require.Equal(t, otellog.KindBool, nested["enabled"].Kind())
	require.True(t, nested["enabled"].AsBool())
	require.Equal(t, otellog.KindSlice, fields["ids"].Kind())
	ids := fields["ids"].AsSlice()
	require.Len(t, ids, 2)
	require.Equal(t, float64(1), authoredLogNumber(t, ids[0]))
	require.Equal(t, float64(2), authoredLogNumber(t, ids[1]))
}
