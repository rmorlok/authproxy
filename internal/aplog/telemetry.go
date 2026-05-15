package aplog

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// otelInstrumentationName is the instrumentation scope reported on log
// records emitted through the OTel logs SDK bridge.
const otelInstrumentationName = "github.com/rmorlok/authproxy/internal/aplog"

// WrapWithTelemetry returns a logger derived from base whose handler:
//
//  1. Always injects trace_id and span_id record attributes when the
//     logger is invoked within a context that carries a recording span.
//     This works regardless of whether telemetry is enabled — the cost is
//     a context lookup per Handle call; the attrs only land when a real
//     trace is in flight.
//
//  2. When telemetry.signals.logs is on AND the providers are live, ALSO
//     fans every record through go.opentelemetry.io/contrib/bridges/otelslog
//     so logs ship via the OTLP logs pipeline alongside traces + metrics.
//     The base handler still emits to its original sink (tint stderr, JSON,
//     etc.) — fanning out is purely additive.
//
// Safe to call with any combination of nil providers / disabled signals /
// nil base — returns base unchanged when there's nothing to add.
//
// The existing CorrelationId field on LogRecord (the request-log entity)
// is unaffected; it carries through independently from trace_id, so logs
// emitted within a traced request carry both.
func WrapWithTelemetry(base *slog.Logger, providers *aptelemetry.Providers, cfg *sconfig.Telemetry) *slog.Logger {
	if base == nil {
		return base
	}

	inner := base.Handler()

	// Always layer the trace-context attrs wrapper. Records emitted outside
	// a traced context fall through unchanged; records emitted inside a
	// recording span gain trace_id / span_id attrs.
	wrapped := &traceContextHandler{inner: inner}

	// When the OTel logs signal is on and providers are live, fan out to
	// the contrib bridge alongside the original sink so logs ship via OTLP.
	if providers != nil && providers.Enabled && cfg.LogsEnabled() {
		otelHandler := otelslog.NewHandler(
			otelInstrumentationName,
			otelslog.WithLoggerProvider(providers.LoggerProvider),
		)
		return slog.New(&multiHandler{handlers: []slog.Handler{wrapped, otelHandler}})
	}

	return slog.New(wrapped)
}

// traceContextHandler injects trace_id / span_id record attributes when the
// supplied context carries a recording span. It is intentionally a thin
// passthrough — no attribute rewriting, no level filtering, no formatting
// changes. Records emitted outside a traced context are forwarded to the
// inner handler unchanged.
type traceContextHandler struct {
	inner slog.Handler
}

// Enabled defers to the inner handler — we never raise or lower the
// effective level.
func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle stamps trace_id / span_id on the record before delegating. The
// span lookup uses trace.SpanContextFromContext (no SDK dependency) so the
// handler is safe to use with no-op providers.
func (h *traceContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record = record.Clone()
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, record)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{inner: h.inner.WithGroup(name)}
}

// multiHandler fans every record / WithAttrs / WithGroup call out to every
// configured inner handler. Errors from one handler don't short-circuit the
// others — a transient failure on the OTLP logs path must not break stderr
// logging, and vice versa.
type multiHandler struct {
	handlers []slog.Handler
}

// Enabled returns true if any inner handler considers the level enabled.
// This avoids dropping records bound for one handler because another has
// a higher threshold.
func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, inner := range h.handlers {
		if inner.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, inner := range h.handlers {
		if !inner.Enabled(ctx, record.Level) {
			continue
		}
		if err := inner.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	wrapped := make([]slog.Handler, len(h.handlers))
	for i, inner := range h.handlers {
		wrapped[i] = inner.WithAttrs(attrs)
	}
	return &multiHandler{handlers: wrapped}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	wrapped := make([]slog.Handler, len(h.handlers))
	for i, inner := range h.handlers {
		wrapped[i] = inner.WithGroup(name)
	}
	return &multiHandler{handlers: wrapped}
}
