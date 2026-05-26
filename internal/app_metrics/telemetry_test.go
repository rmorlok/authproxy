package app_metrics

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// requestLogTelemetryFixture wires an OTel SDK with an in-memory span
// recorder + a manual metric reader so app_metrics telemetry tests can
// introspect emitted telemetry without a live exporter.
type requestLogTelemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newRequestEventsTelemetryFixture(t *testing.T) *requestLogTelemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &requestLogTelemetryFixture{
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

func (f *requestLogTelemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

// newSqliteRequestEventsConfig builds an HTTP-logging config backed by a fresh
// SQLite file. The file is cleaned up at test end.
func newSqliteRequestEventsConfig(t *testing.T) *sconfig.Database {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "authproxy_app_metrics_otel_*.db")
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })

	return &sconfig.Database{
		InnerVal: &sconfig.DatabaseSqlite{Path: tmpFile.Name()},
	}
}

func enabledPtr(b bool) *bool { return &b }

// runProbeQuery issues a trivial SELECT through the store's underlying *sql.DB.
// otelsql instruments Query/Exec/Prepare paths by default; Ping is gated
// behind a SpanOptions flag we don't enable, so a real query is what proves
// the wrapper is wired through.
func runProbeQuery(t *testing.T, store RecordStore) {
	t.Helper()
	concrete, ok := store.(*sqlRecordStore)
	require.True(t, ok, "telemetry test expects the concrete sqlRecordStore type")

	rows, err := concrete.db.QueryContext(context.Background(), "SELECT 1")
	require.NoError(t, err)
	require.NoError(t, rows.Close())
}

func TestRequestEvents_SqlStoreEmitsSpansWhenTelemetryEnabled(t *testing.T) {
	fx := newRequestEventsTelemetryFixture(t)
	tel := &sconfig.Telemetry{Enabled: enabledPtr(true)}

	cfg := newSqliteRequestEventsConfig(t)
	store := NewSqlRecordStore(cfg, newNoopLogger(), database.WithTelemetry(fx.providers, tel))

	runProbeQuery(t, store)

	require.NotEmpty(t, fx.spans.Ended(), "the request-events SQL store must emit spans when telemetry is on")
	rm := fx.readMetrics(t)
	require.NotEmpty(t, rm.ScopeMetrics, "the request-events SQL store must emit metrics when telemetry is on")
}

func TestRequestEvents_SqlStoreNoOpWhenTelemetryDisabled(t *testing.T) {
	fx := newRequestEventsTelemetryFixture(t)

	cfg := newSqliteRequestEventsConfig(t)
	// No WithTelemetry option → constructor takes the plain sql.Open fast
	// path, identical to the historic behaviour.
	store := NewSqlRecordStore(cfg, newNoopLogger())

	runProbeQuery(t, store)

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}
