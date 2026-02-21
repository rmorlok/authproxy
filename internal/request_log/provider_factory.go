package request_log

import (
	"fmt"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

// NewEntryRecordStore creates an EntryRecordStore based on the HttpLogging configuration.
// If no database config is specified, defaults to the Redis provider.
func NewEntryRecordStore(httpLogging *config.HttpLogging, r apredis.Client, logger *slog.Logger) EntryRecordStore {
	provider := httpLogging.Database.GetProvider()

	switch provider {
	case config.DatabaseProviderSqlite:
		return NewSqlEntryRecordStore(httpLogging.GetDatabase(), logger)
	case config.DatabaseProviderPostgres:
		return NewSqlEntryRecordStore(httpLogging.GetDatabase(), logger)
	case config.DatabaseProviderClickhouse:
		return NewClickhouseEntryRecordStore(httpLogging.GetDatabase(), logger)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}
}

// NewEntryRecordRetriever creates an EntryRecordRetriever based on the HttpLogging configuration.
// If no database config is specified, defaults to the Redis provider.
func NewEntryRecordRetriever(httpLogging *config.HttpLogging, r apredis.Client, cursorKey config.KeyDataType, logger *slog.Logger) EntryRecordRetriever {
	provider := httpLogging.GetDatabaseProvider()

	switch provider {
	case config.HttpLoggingDatabaseProviderSqlite:
		return NewSqlEntryRecordRetriever(httpLogging.GetDatabase(), cursorKey, logger)
	case config.HttpLoggingDatabaseProviderPostgres:
		return NewSqlEntryRecordRetriever(httpLogging.GetDatabase(), cursorKey, logger)
	case config.HttpLoggingDatabaseProviderClickhouse:
		return NewClickhouseEntryRecordRetriever(httpLogging.GetDatabase(), cursorKey, logger)
	default:
		panic(fmt.Sprintf("unknown http logging database provider: %s", provider))
	}
}
