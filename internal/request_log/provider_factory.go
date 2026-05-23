package request_log

import (
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// dbSystemFor returns the otel semconv db.system value for an HTTP-logging
// provider. Used to tag spans + metrics emitted by the instrumented DB
// connection so dashboards can break out request-log SQL activity by engine.
func dbSystemFor(p config.DatabaseProvider) string {
	switch p {
	case config.DatabaseProviderPostgres:
		return database.DBSystemPostgreSQL
	case config.DatabaseProviderSqlite:
		return database.DBSystemSQLite
	case config.DatabaseProviderClickhouse:
		return database.DBSystemClickHouse
	default:
		return ""
	}
}

// NewRecordStore creates an RecordStore based on the HttpLogging configuration.
// dbOpts are forwarded to the underlying instrumented DB constructor —
// callers pass database.WithTelemetry(...) to enable spans + metrics on the
// request-log database tier.
func NewRecordStore(cfg *config.Database, logger *slog.Logger, dbOpts ...database.Option) RecordStore {
	provider := cfg.GetProvider()

	switch provider {
	case config.DatabaseProviderSqlite:
		return NewSqlRecordStore(cfg, logger, dbOpts...)
	case config.DatabaseProviderPostgres:
		return NewSqlRecordStore(cfg, logger, dbOpts...)
	case config.DatabaseProviderClickhouse:
		return NewClickhouseRecordStore(cfg, logger, dbOpts...)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}
}

// NewRecordRetriever creates an RecordRetriever based on the HttpLogging configuration.
func NewRecordRetriever(cfg *config.Database, cursorEncryptor pagination.CursorEncryptor, logger *slog.Logger, dbOpts ...database.Option) RecordRetriever {
	provider := cfg.GetProvider()
	switch provider {
	case config.DatabaseProviderSqlite:
		return NewSqlRecordRetriever(cfg, cursorEncryptor, logger, dbOpts...)
	case config.DatabaseProviderPostgres:
		return NewSqlRecordRetriever(cfg, cursorEncryptor, logger, dbOpts...)
	case config.DatabaseProviderClickhouse:
		return NewClickhouseRecordRetriever(cfg, cursorEncryptor, logger, dbOpts...)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}
}
