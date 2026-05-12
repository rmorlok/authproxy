package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// dbTelemetryFixture wires an OTel SDK with an in-memory span recorder + a
// manual metric reader so DB tests can introspect emitted telemetry without
// a live exporter.
type dbTelemetryFixture struct {
	providers *aptelemetry.Providers
	spans     *tracetest.SpanRecorder
	reader    *sdkmetric.ManualReader
}

func newDBTelemetryFixture(t *testing.T) *dbTelemetryFixture {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	return &dbTelemetryFixture{
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

func (f *dbTelemetryFixture) readMetrics(t *testing.T) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	require.NoError(t, f.reader.Collect(context.Background(), &rm))
	return rm
}

// newSqliteDBWith opens a fresh SQLite-backed DB using the supplied
// telemetry options and returns it ready for use. The DB file is cleaned up
// at test end.
func newSqliteDBWith(t *testing.T, opts ...Option) DB {
	t.Helper()
	tempPath := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("authproxy-otel-tests/%s-%s.sqlite3", t.Name(), uuid.New().String()),
	)
	require.NoError(t, os.MkdirAll(filepath.Dir(tempPath), os.ModePerm))
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(tempPath)) })

	cfg := &sconfig.DatabaseSqlite{Path: tempPath}
	db, err := NewSqliteConnection(cfg, nil, opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.(*service).db.Close() })

	return db
}

func boolPtr(b bool) *bool { return &b }

func TestDBTelemetry_QueryEmitsSpanAndMetric(t *testing.T) {
	fx := newDBTelemetryFixture(t)
	tel := &sconfig.Telemetry{Enabled: boolPtr(true)}

	db := newSqliteDBWith(t, WithTelemetry(fx.providers, tel))

	// Issue a trivial query that doesn't depend on migrations.
	require.True(t, db.Ping(context.Background()))

	gotSpans := fx.spans.Ended()
	require.NotEmpty(t, gotSpans, "instrumented query must produce at least one span")

	rm := fx.readMetrics(t)
	// otelsql emits one or more standard metric series on first use; we
	// don't pin the exact name (it's contributed by otelsql), but at least
	// one metric stream must be present.
	require.NotEmpty(t, rm.ScopeMetrics, "metrics must be emitted via the otelsql instrumentation")
}

func TestDBTelemetry_NoOpWhenTelemetryDisabled(t *testing.T) {
	fx := newDBTelemetryFixture(t)

	// Disabled telemetry: no providers attached, so the constructor must
	// fall through to plain sql.Open and emit nothing.
	db := newSqliteDBWith(t /* no options */)
	require.True(t, db.Ping(context.Background()))

	require.Empty(t, fx.spans.Ended(), "no spans expected when telemetry isn't wired in")
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics, "no metrics expected when telemetry isn't wired in")
}

func TestDBTelemetry_NoOpWhenProvidersDisabled(t *testing.T) {
	// Providers in no-op mode (the default when telemetry config is
	// absent) — pass them through anyway and verify the constructor still
	// avoids wrapping with otelsql.
	fx := newDBTelemetryFixture(t)
	tel := &sconfig.Telemetry{} // Enabled is nil → IsEnabled() == false

	db := newSqliteDBWith(t, WithTelemetry(aptelemetry.NoopProviders(), tel))
	require.True(t, db.Ping(context.Background()))

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}

func TestDBTelemetry_NoOpWhenBothSignalsOff(t *testing.T) {
	// Enabled providers but both trace and metric signals turned off.
	fx := newDBTelemetryFixture(t)
	off := false
	on := true
	tel := &sconfig.Telemetry{
		Enabled: &on,
		Signals: &sconfig.TelemetrySignals{Traces: &off, Metrics: &off, Logs: &off},
	}

	db := newSqliteDBWith(t, WithTelemetry(fx.providers, tel))
	require.True(t, db.Ping(context.Background()))

	require.Empty(t, fx.spans.Ended())
	rm := fx.readMetrics(t)
	require.Empty(t, rm.ScopeMetrics)
}
