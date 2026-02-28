package request_log

import (
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/schema/config"
)

// NewRecordStore creates an RecordStore based on the HttpLogging configuration.
// If no database config is specified, defaults to the Redis provider.
func NewRecordStore(database *config.Database, logger *slog.Logger) RecordStore {
	provider := database.GetProvider()

	switch provider {
	case config.DatabaseProviderSqlite:
		return NewSqlRecordStore(database, logger)
	case config.DatabaseProviderPostgres:
		return NewSqlRecordStore(database, logger)
	case config.DatabaseProviderClickhouse:
		return NewClickhouseRecordStore(database, logger)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}
}

// NewRecordRetriever creates an RecordRetriever based on the HttpLogging configuration.
// If no database config is specified, defaults to the Redis provider.
func NewRecordRetriever(database *config.Database, cursorKey config.KeyDataType, logger *slog.Logger) RecordRetriever {
	provider := database.GetProvider()
	switch provider {
	case config.DatabaseProviderSqlite:
	case config.DatabaseProviderPostgres:
		return NewSqlRecordRetriever(database, cursorKey, logger)
	case config.DatabaseProviderClickhouse:
		return NewClickhouseRecordRetriever(database, cursorKey, logger)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}

	panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
}
