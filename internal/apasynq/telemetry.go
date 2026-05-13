package apasynq

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryInstrumentationName is the instrumentation scope reported on the
// emitted spans and metrics.
const telemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/apasynq"

// taskResultSuccess / taskResultError are the bounded-cardinality values for
// the result dimension on task metrics. retry_count is exposed as a span
// attribute (high-fanout if used as a metric dim).
const (
	taskResultSuccess = "success"
	taskResultError   = "error"
)

// Telemetry bundles the asynq instrumentation surface for the worker
// pipeline: a handler middleware to wrap mux registrations, a wrapper for
// the scheduler's GetConfigs hook so each scheduler sync is observable, and
// an observable-gauge factory for queue depth polling.
//
// All entry points are safe to call when telemetry is disabled — the
// middleware passes through, the scheduler wrapper invokes the inner
// function without spanning, and StartQueueDepthGauge returns a no-op stop
// function.
type Telemetry struct {
	tracesEnabled  bool
	metricsEnabled bool
	tracer         trace.Tracer
	meter          metric.Meter

	taskDuration metric.Float64Histogram

	inspector *asynq.Inspector
}

// NewTelemetry constructs a Telemetry surface from the providers + config.
// providers being nil or in no-op mode, or both signals being off, produces
// a Telemetry whose methods are inert — callers don't need to gate on this
// themselves.
func NewTelemetry(providers *aptelemetry.Providers, cfg *sconfig.Telemetry, inspector *asynq.Inspector) (*Telemetry, error) {
	t := &Telemetry{inspector: inspector}

	if providers == nil || !providers.Enabled {
		return t, nil
	}

	t.tracesEnabled = cfg.TracesEnabled()
	t.metricsEnabled = cfg.MetricsEnabled()
	if !t.tracesEnabled && !t.metricsEnabled {
		return t, nil
	}

	if t.tracesEnabled {
		t.tracer = providers.TracerProvider.Tracer(telemetryInstrumentationName)
	}

	if t.metricsEnabled {
		t.meter = providers.MeterProvider.Meter(telemetryInstrumentationName)

		var err error
		t.taskDuration, err = t.meter.Float64Histogram(
			"authproxy.asynq.task.duration",
			metric.WithUnit("s"),
			metric.WithDescription("Duration of asynq task handler invocations."),
		)
		if err != nil {
			return nil, fmt.Errorf("apasynq: create task duration histogram: %w", err)
		}
	}

	return t, nil
}

// active reports whether at least one signal is enabled. Used to short-
// circuit the hot paths in middleware / wrappers when telemetry is off.
// Safe to call on a nil receiver.
func (t *Telemetry) active() bool {
	return t != nil && (t.tracesEnabled || t.metricsEnabled)
}

// tracingActive reports whether trace emission is enabled. Safe on nil.
func (t *Telemetry) tracingActive() bool {
	return t != nil && t.tracesEnabled
}

// Middleware returns an asynq.MiddlewareFunc that opens a span around each
// handler invocation and records the duration histogram on exit. When
// telemetry is disabled the returned middleware is the identity wrapper —
// it forwards to the next handler with no extra work. Safe to call on a
// nil receiver.
func (t *Telemetry) Middleware() asynq.MiddlewareFunc {
	if !t.active() {
		return func(next asynq.Handler) asynq.Handler { return next }
	}

	return func(next asynq.Handler) asynq.Handler {
		return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) error {
			taskType := task.Type()
			queue := stringFromContext(ctx, asynq.GetQueueName, "")
			taskID := stringFromContext(ctx, asynq.GetTaskID, "")
			retry := intFromContext(ctx, asynq.GetRetryCount, 0)
			maxRetry := intFromContext(ctx, asynq.GetMaxRetry, 0)

			var span trace.Span
			if t.tracesEnabled {
				attrs := []attribute.KeyValue{
					attribute.String("messaging.system", "asynq"),
					attribute.String("messaging.destination.name", queue),
					attribute.String("messaging.operation.type", "process"),
					attribute.String("messaging.message.id", taskID),
					attribute.String("authproxy.asynq.task_type", taskType),
					attribute.Int("authproxy.asynq.retry_count", retry),
					attribute.Int("authproxy.asynq.max_retry", maxRetry),
				}
				ctx, span = t.tracer.Start(
					ctx,
					"asynq.task "+taskType,
					trace.WithSpanKind(trace.SpanKindConsumer),
					trace.WithAttributes(attrs...),
				)
				defer span.End()
			}

			start := time.Now()
			err := next.ProcessTask(ctx, task)
			elapsed := time.Since(start)

			result := taskResultSuccess
			if err != nil {
				result = taskResultError
				if span != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				}
			}

			if t.metricsEnabled {
				metricAttrs := []attribute.KeyValue{
					attribute.String("authproxy.asynq.task_type", taskType),
					attribute.String("messaging.destination.name", queue),
					attribute.String("authproxy.asynq.result", result),
				}
				t.taskDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(metricAttrs...))
			}

			return err
		})
	}
}

// WithSchedulerSyncSpan wraps a scheduler sync (called periodically by
// asynq.PeriodicTaskManager via the PeriodicTaskConfigProvider interface)
// in a span. Errors are recorded on the span. When telemetry is disabled
// the wrapper invokes fn directly with no span overhead. Safe to call on a
// nil receiver.
func (t *Telemetry) WithSchedulerSyncSpan(ctx context.Context, fn func() ([]*asynq.PeriodicTaskConfig, error)) ([]*asynq.PeriodicTaskConfig, error) {
	if !t.tracingActive() {
		return fn()
	}

	_, span := t.tracer.Start(
		ctx,
		"asynq.scheduler.sync",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	configs, err := fn()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int("authproxy.asynq.scheduler.task_count", len(configs)))
	return configs, nil
}

// StartQueueDepthGauge registers an observable-gauge callback that, on every
// metric collection, polls the asynq Inspector for the size of each named
// queue and emits an observation. Returns a stop function that unregisters
// the callback; safe to defer.
//
// queues is the list of queue names to track. When metrics are disabled or
// no inspector was supplied, returns a no-op stop function and registers
// nothing.
func (t *Telemetry) StartQueueDepthGauge(queues []string) (stop func(), err error) {
	if t == nil || !t.metricsEnabled || t.inspector == nil || len(queues) == 0 {
		return func() {}, nil
	}

	gauge, err := t.meter.Int64ObservableGauge(
		"authproxy.asynq.queue.size",
		metric.WithDescription("Number of pending tasks in an asynq queue."),
	)
	if err != nil {
		return nil, fmt.Errorf("apasynq: create queue size gauge: %w", err)
	}

	reg, err := t.meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			for _, q := range queues {
				info, err := t.inspector.GetQueueInfo(q)
				if err != nil {
					// Best-effort: a transient inspector failure must
					// not break the metric pipeline.
					continue
				}
				observer.ObserveInt64(
					gauge,
					int64(info.Size),
					metric.WithAttributes(attribute.String("messaging.destination.name", q)),
				)
			}
			return nil
		},
		gauge,
	)
	if err != nil {
		return nil, fmt.Errorf("apasynq: register queue size callback: %w", err)
	}

	return func() { _ = reg.Unregister() }, nil
}

// stringFromContext applies the supplied asynq getter and returns its result,
// or fallback when the getter didn't find a value in the context.
func stringFromContext(ctx context.Context, getter func(context.Context) (string, bool), fallback string) string {
	v, ok := getter(ctx)
	if !ok || v == "" {
		return fallback
	}
	return v
}

// intFromContext applies the supplied asynq int-getter, returning fallback when
// no value is present in the context.
func intFromContext(ctx context.Context, getter func(context.Context) (int, bool), fallback int) int {
	v, ok := getter(ctx)
	if !ok {
		return fallback
	}
	return v
}
