package request_log

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
)

// RecordStore handles persisting LogRecord metadata to a storage backend.
type RecordStore interface {
	// StoreRecord persists a LogRecord to the storage backend.
	StoreRecord(ctx context.Context, record *LogRecord) error

	// StoreRecords persists multiple LogRecord to the storage backend.
	StoreRecords(ctx context.Context, records []*LogRecord) error
}

type migratable interface {
	// Migrate runs any necessary schema migrations for the storage backend.
	Migrate(ctx context.Context) error
}

// RecordRetriever handles querying LogRecord metadata from a storage backend.
type RecordRetriever interface {
	// GetRecord retrieves a single LogRecord by its request ID.
	GetRecord(ctx context.Context, id apid.ID) (*LogRecord, error)

	// NewListRequestsBuilder creates a new builder for listing entry records with filters.
	NewListRequestsBuilder() ListRequestBuilder

	// ListRequestsFromCursor resumes a paginated listing from a cursor string.
	ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error)
}
