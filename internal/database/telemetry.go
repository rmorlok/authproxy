package database

import (
	"database/sql"
	"database/sql/driver"

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
// (e.g. "postgresql").
func openInstrumentedDB(driverName, dsn, dbSystem string, opts *telemetryOpts) (*sql.DB, error) {
	if !opts.telemetryEnabled() {
		return sql.Open(driverName, dsn)
	}

	otelOpts := otelOptionsFor(opts, dbSystem)
	db, err := otelsql.Open(driverName, dsn, otelOpts...)
	if err != nil {
		return nil, err
	}
	maybeRegisterPoolStats(opts, db, otelOpts)
	return db, nil
}

// OpenInstrumentedSQL is the exported counterpart of openInstrumentedDB.
// Other packages that own their own sql.DB constructors (e.g. request_log,
// which opens separate Postgres / SQLite connections for HTTP request
// logging) call this to inherit the same telemetry treatment as the main
// database without duplicating the plumbing.
//
// dbSystem is the semconv db.system value (DBSystemPostgreSQL, DBSystemSQLite,
// DBSystemClickHouse). opts are forwarded as-is — pass WithTelemetry to
// instrument; pass nothing for a plain sql.Open fast path.
func OpenInstrumentedSQL(driverName, dsn, dbSystem string, opts ...Option) (*sql.DB, error) {
	return openInstrumentedDB(driverName, dsn, dbSystem, resolveOpts(opts))
}

// OpenInstrumentedConnector wraps a driver.Connector — used by drivers that
// expose a Connector rather than a registered driver name. The ClickHouse
// std driver is the in-tree example (clickhouse.Connector(opts) returns a
// driver.Connector that can be wrapped here). When telemetry is disabled,
// returns a plain sql.OpenDB(connector) with no overhead.
func OpenInstrumentedConnector(connector driver.Connector, dbSystem string, opts ...Option) *sql.DB {
	resolved := resolveOpts(opts)
	if !resolved.telemetryEnabled() {
		return sql.OpenDB(connector)
	}

	otelOpts := otelOptionsFor(resolved, dbSystem)
	db := otelsql.OpenDB(connector, otelOpts...)
	maybeRegisterPoolStats(resolved, db, otelOpts)
	return db
}

// otelOptionsFor builds the otelsql option slice common to both the
// driver-name and connector paths.
func otelOptionsFor(opts *telemetryOpts, dbSystem string) []otelsql.Option {
	return []otelsql.Option{
		otelsql.WithTracerProvider(opts.providers.TracerProvider),
		otelsql.WithMeterProvider(opts.providers.MeterProvider),
		otelsql.WithAttributes(attribute.String(string(semconv.DBSystemKey), dbSystem)),
	}
}

// maybeRegisterPoolStats registers the standard go-sql DB connection pool
// gauges (open / in-use / idle, wait counts) when metrics are enabled.
// Failures are non-fatal — instrumentation is best effort.
func maybeRegisterPoolStats(opts *telemetryOpts, db *sql.DB, otelOpts []otelsql.Option) {
	if !opts.cfg.MetricsEnabled() {
		return
	}
	_, _ = otelsql.RegisterDBStatsMetrics(db, otelOpts...)
}

// DBSystemPostgreSQL / DBSystemSQLite / DBSystemClickHouse are the
// otel semconv db.system values used when constructing instrumented
// connections. Pass these to OpenInstrumentedSQL / OpenInstrumentedConnector.
const (
	DBSystemPostgreSQL = "postgresql"
	DBSystemSQLite     = "sqlite"
	DBSystemClickHouse = "clickhouse"
)
