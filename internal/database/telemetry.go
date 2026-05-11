package database

import (
	"database/sql"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/rmorlok/authproxy/internal/aptelemetry"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// telemetryOpts holds the resolved OTel providers + config used when opening a
// sql.DB. nil providers, providers in no-op mode, or both signals disabled
// degrade to plain sql.Open with no overhead.
type telemetryOpts struct {
	providers *aptelemetry.Providers
	cfg       *sconfig.Telemetry
}

// Option configures connection setup. WithTelemetry is currently the only
// option; the functional-options shape keeps the constructor signatures
// non-breaking for existing callers (e.g. test_db.go) that don't need
// telemetry.
type Option func(*telemetryOpts)

// WithTelemetry causes the constructed sql.DB to be instrumented with OTel
// spans + metrics via otelsql. When providers is nil or in no-op mode, or
// when both trace and metric signals are off in cfg, this is silently inert
// and a plain sql.DB is returned.
func WithTelemetry(providers *aptelemetry.Providers, cfg *sconfig.Telemetry) Option {
	return func(o *telemetryOpts) {
		o.providers = providers
		o.cfg = cfg
	}
}

// resolveOpts collapses zero or more Options into the effective settings.
func resolveOpts(opts []Option) *telemetryOpts {
	o := &telemetryOpts{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// telemetryEnabled reports whether the resolved telemetry options should
// produce an instrumented sql.DB. Both a live providers handle and at least
// one enabled signal are required.
func (o *telemetryOpts) telemetryEnabled() bool {
	if o == nil || o.providers == nil || !o.providers.Enabled {
		return false
	}
	return o.cfg.TracesEnabled() || o.cfg.MetricsEnabled()
}

// openInstrumentedDB opens a sql.DB connection, wrapping it with otelsql when
// telemetry is enabled and falling through to plain sql.Open otherwise.
// driverName is the underlying driver registered with database/sql (e.g.
// "pgx", "sqlite3"); dbSystem is the OTel semconv db.system attribute value
// (e.g. semconv.DBSystemPostgreSQL.Value.AsString()).
func openInstrumentedDB(driverName, dsn, dbSystem string, opts *telemetryOpts) (*sql.DB, error) {
	if !opts.telemetryEnabled() {
		return sql.Open(driverName, dsn)
	}

	otelOpts := []otelsql.Option{
		otelsql.WithTracerProvider(opts.providers.TracerProvider),
		otelsql.WithMeterProvider(opts.providers.MeterProvider),
		otelsql.WithAttributes(attribute.String(string(semconv.DBSystemKey), dbSystem)),
	}

	db, err := otelsql.Open(driverName, dsn, otelOpts...)
	if err != nil {
		return nil, err
	}

	if opts.cfg.MetricsEnabled() {
		// Register go.sql.DB connection pool gauges (open / in-use / idle
		// connections, wait counts, etc.). Ignore the returned values: a
		// failure to register pool stats should not fail the whole
		// connection — instrumentation is best-effort.
		_, _ = otelsql.RegisterDBStatsMetrics(db, otelOpts...)
	}

	return db, nil
}

// dbSystemPostgres is the otel semconv db.system value for Postgres.
const dbSystemPostgres = "postgresql"

// dbSystemSQLite is the otel semconv db.system value for SQLite.
const dbSystemSQLite = "sqlite"
