package oauth2

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryInstrumentationName is the instrumentation scope reported on the
// emitted spans and metrics.
const telemetryInstrumentationName = "github.com/rmorlok/authproxy/internal/auth_methods/oauth2"

// Result values for the result dimension on lifecycle counters. Kept bounded
// — operators alert on these strings.
const (
	resultSuccess = "success"
	resultFailure = "failure"
)

// Revocation kinds — emitted on the revocation counter so dashboards can
// break out refresh-token vs access-token revocations.
const (
	revocationKindRefresh = "refresh_token"
	revocationKindAccess  = "access_token"
)

// telemetry bundles the OAuth2 lifecycle instrumentation surface: tracer +
// counters for token-exchange, refresh, and revocation. Every method is
// nil-safe so the call sites can hold an optional *telemetry without
// guarding on each use.
type telemetry struct {
	tracesEnabled  bool
	metricsEnabled bool
	tracer         trace.Tracer

	refreshAttempts        metric.Int64Counter
	refreshFailures        metric.Int64Counter
	revocations            metric.Int64Counter
	tokenExchangeAttempts  metric.Int64Counter
	tokenExchangeFailures  metric.Int64Counter
}

// newTelemetry constructs a telemetry surface from the providers + config.
// nil providers, providers in no-op mode, or both signals off produce a
// telemetry whose methods are inert.
func newTelemetry(providers *aptelemetry.Providers, cfg *sconfig.Telemetry) (*telemetry, error) {
	t := &telemetry{}

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
		meter := providers.MeterProvider.Meter(telemetryInstrumentationName)

		var err error
		if t.refreshAttempts, err = meter.Int64Counter(
			"authproxy.oauth2.refresh.attempts.total",
			metric.WithDescription("OAuth2 refresh-token grant attempts, by result and connector."),
		); err != nil {
			return nil, fmt.Errorf("oauth2: create refresh attempts counter: %w", err)
		}
		if t.refreshFailures, err = meter.Int64Counter(
			"authproxy.oauth2.refresh.failures.total",
			metric.WithDescription("OAuth2 refresh-token grant failures, by reason and connector."),
		); err != nil {
			return nil, fmt.Errorf("oauth2: create refresh failures counter: %w", err)
		}
		if t.revocations, err = meter.Int64Counter(
			"authproxy.oauth2.revocations.total",
			metric.WithDescription("OAuth2 token revocations, by kind, result, and connector."),
		); err != nil {
			return nil, fmt.Errorf("oauth2: create revocations counter: %w", err)
		}
		if t.tokenExchangeAttempts, err = meter.Int64Counter(
			"authproxy.oauth2.token_exchange.attempts.total",
			metric.WithDescription("OAuth2 authorization-code → token exchange attempts, by result and connector."),
		); err != nil {
			return nil, fmt.Errorf("oauth2: create token exchange attempts counter: %w", err)
		}
		if t.tokenExchangeFailures, err = meter.Int64Counter(
			"authproxy.oauth2.token_exchange.failures.total",
			metric.WithDescription("OAuth2 authorization-code → token exchange failures, by reason and connector."),
		); err != nil {
			return nil, fmt.Errorf("oauth2: create token exchange failures counter: %w", err)
		}
	}

	return t, nil
}

// tracingActive reports whether traces are enabled. Safe to call on nil.
func (t *telemetry) tracingActive() bool {
	return t != nil && t.tracesEnabled
}

// metricsActive reports whether metrics are enabled. Safe to call on nil.
func (t *telemetry) metricsActive() bool {
	return t != nil && t.metricsEnabled
}

// connectorAttr returns the connector_id attribute or an empty slice when
// the id is unset. Connector id is bounded-cardinality (one per connector
// version row), so it's a safe metric dimension.
func connectorAttr(connectorID apid.ID) []attribute.KeyValue {
	if connectorID.IsNil() {
		return nil
	}
	return []attribute.KeyValue{attribute.String("authproxy.connector_id", connectorID.String())}
}

// withSpan wraps fn in a span when tracing is active. The span ends on
// return; errors are recorded and the span status set to Error. When
// tracing is off, fn is invoked directly with no overhead.
func (t *telemetry) withSpan(
	ctx context.Context,
	name string,
	connectorID apid.ID,
	fn func(ctx context.Context) error,
) error {
	if !t.tracingActive() {
		return fn(ctx)
	}

	attrs := connectorAttr(connectorID)
	attrs = append(attrs, attribute.String("authproxy.oauth2.operation", name))

	ctx, span := t.tracer.Start(
		ctx,
		"oauth2."+name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

// recordRefreshSuccess increments the refresh-attempts counter with
// result=success.
func (t *telemetry) recordRefreshSuccess(ctx context.Context, connectorID apid.ID) {
	if !t.metricsActive() {
		return
	}
	attrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.result", resultSuccess))
	t.refreshAttempts.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordRefreshFailure increments the refresh-attempts counter with
// result=failure and the refresh-failures counter with reason=<category>.
// Both counters carry connector_id so dashboards can break failures down
// by connector.
func (t *telemetry) recordRefreshFailure(ctx context.Context, reason string, connectorID apid.ID) {
	if !t.metricsActive() {
		return
	}

	attemptAttrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.result", resultFailure))
	t.refreshAttempts.Add(ctx, 1, metric.WithAttributes(attemptAttrs...))

	failureAttrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.reason", reason))
	t.refreshFailures.Add(ctx, 1, metric.WithAttributes(failureAttrs...))
}

// recordRevocation increments the revocations counter with kind +
// success / failure outcome.
func (t *telemetry) recordRevocation(ctx context.Context, kind string, ok bool, connectorID apid.ID) {
	if !t.metricsActive() {
		return
	}
	result := resultSuccess
	if !ok {
		result = resultFailure
	}
	attrs := append(connectorAttr(connectorID),
		attribute.String("authproxy.oauth2.revocation_kind", kind),
		attribute.String("authproxy.oauth2.result", result),
	)
	t.revocations.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// recordTokenExchangeSuccess / recordTokenExchangeFailure mirror the refresh
// pair — attempts with result, failures with reason. The "attempts" counter
// lets dashboards graph success rate without separately tracking successes.
func (t *telemetry) recordTokenExchangeSuccess(ctx context.Context, connectorID apid.ID) {
	if !t.metricsActive() {
		return
	}
	attrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.result", resultSuccess))
	t.tokenExchangeAttempts.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func (t *telemetry) recordTokenExchangeFailure(ctx context.Context, reason string, connectorID apid.ID) {
	if !t.metricsActive() {
		return
	}

	attemptAttrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.result", resultFailure))
	t.tokenExchangeAttempts.Add(ctx, 1, metric.WithAttributes(attemptAttrs...))

	failureAttrs := append(connectorAttr(connectorID), attribute.String("authproxy.oauth2.reason", reason))
	t.tokenExchangeFailures.Add(ctx, 1, metric.WithAttributes(failureAttrs...))
}
