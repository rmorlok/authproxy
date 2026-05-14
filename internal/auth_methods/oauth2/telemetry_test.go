package oauth2

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// attrKey wraps attribute.Key for terse use in test assertions on
// attribute.Set.Value (which takes attribute.Key, not string).
func attrKey(k string) attribute.Key { return attribute.Key(k) }

// oauth2TelemetryFixture wires an OTel SDK with an in-memory span recorder +
// a manual metric reader so the lifecycle telemetry tests can introspect
// emitted telemetry without a live exporter.
type oauth2TelemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newOauth2TelemetryFixture(t *testing.T) *oauth2TelemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &oauth2TelemetryFixture{
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

func (f *oauth2TelemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

func enabledPtr(b bool) *bool { return &b }

func TestTelemetry_RefreshSuccessAndFailureCounters(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	tel.recordRefreshSuccess(context.Background(), nil)
	tel.recordRefreshFailure(context.Background(), string(tokenRefreshInvalidGrant), nil)
	tel.recordRefreshFailure(context.Background(), string(tokenRefreshProvider5xx), nil)

	rm := fx.readMetrics(t)
	requireMetricEmitted(t, rm, "authproxy.oauth2.refresh.attempts.total")
	requireMetricEmitted(t, rm, "authproxy.oauth2.refresh.failures.total")
}

func TestTelemetry_RevocationCounter(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	tel.recordRevocation(context.Background(), revocationKindRefresh, true, nil)
	tel.recordRevocation(context.Background(), revocationKindAccess, false, nil)

	rm := fx.readMetrics(t)
	requireMetricEmitted(t, rm, "authproxy.oauth2.revocations.total")
}

func TestTelemetry_TokenExchangeCounters(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	tel.recordTokenExchangeSuccess(context.Background(), nil)
	tel.recordTokenExchangeFailure(context.Background(), string(tokenExchangeInvalidGrant), nil)

	rm := fx.readMetrics(t)
	requireMetricEmitted(t, rm, "authproxy.oauth2.token_exchange.attempts.total")
	requireMetricEmitted(t, rm, "authproxy.oauth2.token_exchange.failures.total")
}

func TestTelemetry_WithSpanRecordsErrors(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	connectorID := apid.New(apid.PrefixConnectorVersion)

	// Success path leaves span status unset.
	require.NoError(t, tel.withSpan(context.Background(), "refresh", connectorID, func(ctx context.Context) error {
		return nil
	}))
	got := fx.spans.Ended()
	require.Len(t, got, 1)
	require.Equal(t, "oauth2.refresh", got[0].Name())
	require.Equal(t, codes.Unset, got[0].Status().Code)

	// Error path marks span errored and records the error.
	wantErr := errors.New("boom")
	err = tel.withSpan(context.Background(), "revoke", connectorID, func(ctx context.Context) error {
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	got = fx.spans.Ended()
	require.Len(t, got, 2)
	last := got[1]
	require.Equal(t, "oauth2.revoke", last.Name())
	require.Equal(t, codes.Error, last.Status().Code)
	require.NotEmpty(t, last.Events(), "RecordError must add an exception event")
}

func TestTelemetry_NoOpWhenProvidersDisabled(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(aptelemetry.NoopProviders(), &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	connectorID := apid.New(apid.PrefixConnectorVersion)
	tel.recordRefreshSuccess(context.Background(), nil)
	tel.recordRefreshFailure(context.Background(), "x", nil)
	tel.recordRevocation(context.Background(), revocationKindRefresh, true, nil)
	tel.recordTokenExchangeSuccess(context.Background(), nil)
	tel.recordTokenExchangeFailure(context.Background(), "y", nil)
	_ = tel.withSpan(context.Background(), "refresh", connectorID, func(ctx context.Context) error { return nil })

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestTelemetry_NoOpWhenAllSignalsOff(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	off := false
	on := true
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	})
	require.NoError(t, err)

	tel.recordRefreshSuccess(context.Background(), nil)
	_ = tel.withSpan(context.Background(), "refresh", apid.Nil, func(ctx context.Context) error { return nil })

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestTelemetry_NilReceiverIsSafe(t *testing.T) {
	// Defensive: every method on *telemetry must be nil-safe so call sites
	// never need to guard. Refresh/exchange/revocation counters + withSpan
	// all exercise the nil-receiver fast path.
	var tel *telemetry

	tel.recordRefreshSuccess(context.Background(), nil)
	tel.recordRefreshFailure(context.Background(), "x", nil)
	tel.recordRevocation(context.Background(), revocationKindRefresh, true, nil)
	tel.recordTokenExchangeSuccess(context.Background(), nil)
	tel.recordTokenExchangeFailure(context.Background(), "y", nil)

	err := tel.withSpan(context.Background(), "refresh", apid.Nil, func(ctx context.Context) error {
		return errors.New("ignored")
	})
	require.Error(t, err, "withSpan must still invoke fn on nil receiver")
}

// TestTelemetry_MetricsExcludeConnectorIDAttribute pins the cardinality
// contract: AuthProxy deployments can have hundreds to thousands of
// connectors, so connector_id MUST NOT appear as a metric dimension. Every
// emitted metric data point's attribute set is checked to confirm
// authproxy.connector_id is absent. Span attributes still carry connector_id
// — that's covered by the design of withSpan (spans aren't a time series).
func TestTelemetry_MetricsExcludeConnectorIDAttribute(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	tel, err := newTelemetry(fx.providers, &sconfig.Telemetry{Enabled: enabledPtr(true)})
	require.NoError(t, err)

	// Provide non-trivial connection labels — none of these are
	// allowlisted, so they MUST also be absent from the metric attributes.
	connectionLabels := map[string]string{
		"type":   "google_drive",
		"env":    "prod",
		"tenant": "acme",
	}

	tel.recordRefreshSuccess(context.Background(), connectionLabels)
	tel.recordRefreshFailure(context.Background(), string(tokenRefreshInvalidGrant), connectionLabels)
	tel.recordRevocation(context.Background(), revocationKindRefresh, true, connectionLabels)
	tel.recordRevocation(context.Background(), revocationKindAccess, false, connectionLabels)
	tel.recordTokenExchangeSuccess(context.Background(), connectionLabels)
	tel.recordTokenExchangeFailure(context.Background(), string(tokenExchangeInvalidGrant), connectionLabels)

	rm := fx.readMetrics(t)
	require.NotEmpty(t, rm.ScopeMetrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "expected Sum[int64] for counter %q, got %T", m.Name, m.Data)
			for _, dp := range sum.DataPoints {
				_, present := dp.Attributes.Value("authproxy.connector_id")
				require.False(t, present,
					"metric %q must not carry authproxy.connector_id (would explode cardinality at scale)",
					m.Name)

				// Allowlist is empty in this test, so connection labels
				// must NOT appear on metrics either — the projector
				// only emits keys that are explicitly allowlisted.
				for _, k := range []string{"type", "env", "tenant"} {
					_, present := dp.Attributes.Value(attrKey(k))
					require.False(t, present,
						"metric %q must not carry unlisted connection label %q",
						m.Name, k)
				}
			}
		}
	}
}

// TestTelemetry_AllowlistedLabelsProject pins the spec from #225 / #232:
// "Counters dimensioned by connector (per allowlist-style projection
// consistent with #232 — only allowlisted labels become dimensions)." The
// projector reads from connection.GetLabels(); allowlisted keys land on
// metric attributes, unlisted keys do not.
func TestTelemetry_AllowlistedLabelsProject(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	on := true
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Proxy: &sconfig.TelemetryProxy{
			MetricDimensionLabels: []string{"type", "env"},
		},
	}
	tel, err := newTelemetry(fx.providers, cfg)
	require.NoError(t, err)

	connectionLabels := map[string]string{
		"type":     "google_drive",
		"env":      "prod",
		"tenant":   "acme", // not allowlisted — must NOT appear
		"internal": "x",    // not allowlisted — must NOT appear
	}

	tel.recordRefreshSuccess(context.Background(), connectionLabels)
	tel.recordRefreshFailure(context.Background(), string(tokenRefreshInvalidGrant), connectionLabels)

	rm := fx.readMetrics(t)
	require.NotEmpty(t, rm.ScopeMetrics)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				gotType, hasType := dp.Attributes.Value(attrKey("type"))
				gotEnv, hasEnv := dp.Attributes.Value(attrKey("env"))
				require.True(t, hasType, "metric %q must carry allowlisted key 'type'", m.Name)
				require.Equal(t, "google_drive", gotType.AsString())
				require.True(t, hasEnv, "metric %q must carry allowlisted key 'env'", m.Name)
				require.Equal(t, "prod", gotEnv.AsString())

				_, hasTenant := dp.Attributes.Value(attrKey("tenant"))
				require.False(t, hasTenant, "metric %q must NOT carry unlisted key 'tenant'", m.Name)
				_, hasInternal := dp.Attributes.Value(attrKey("internal"))
				require.False(t, hasInternal, "metric %q must NOT carry unlisted key 'internal'", m.Name)
			}
		}
	}
}

// TestTelemetry_ValueCapAppliesToProjectedLabels pins that the
// metric_dimension_value_cap from #232 applies to OAuth2 metrics too —
// overflow values collapse to "other" via the shared LabelProjector.
func TestTelemetry_ValueCapAppliesToProjectedLabels(t *testing.T) {
	fx := newOauth2TelemetryFixture(t)
	on := true
	cap := 2
	cfg := &sconfig.Telemetry{
		Enabled: &on,
		Proxy: &sconfig.TelemetryProxy{
			MetricDimensionLabels:   []string{"tenant"},
			MetricDimensionValueCap: &cap,
		},
	}
	tel, err := newTelemetry(fx.providers, cfg)
	require.NoError(t, err)

	// Emit three distinct tenant values — the third must collapse to
	// "other" per the cap semantics.
	for _, tenant := range []string{"a", "b", "c"} {
		tel.recordRefreshSuccess(context.Background(), map[string]string{"tenant": tenant})
	}

	rm := fx.readMetrics(t)
	m := findMetric(t, rm, "authproxy.oauth2.refresh.attempts.total")
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok)

	seen := map[string]bool{}
	for _, dp := range sum.DataPoints {
		v, present := dp.Attributes.Value(attrKey("tenant"))
		require.True(t, present)
		seen[v.AsString()] = true
	}
	require.True(t, seen["a"], "first admitted value passes through verbatim")
	require.True(t, seen["b"], "second admitted value passes through verbatim")
	require.False(t, seen["c"], "third distinct value must collapse to 'other'")
	require.True(t, seen[aptelemetry.LabelValueOther], "overflow value must land in the 'other' bucket")
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
	names := []string{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	t.Fatalf("metric %q was expected but not emitted; got: %v", name, names)
}

func findMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q not emitted", name)
	return metricdata.Metrics{}
}
