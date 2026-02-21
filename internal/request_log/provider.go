package request_log

import (
	"context"

	"github.com/google/uuid"
)

// EntryRecordStore handles persisting EntryRecord metadata to a storage backend.
type EntryRecordStore interface {
	// StoreRecord persists an EntryRecord to the storage backend.
	StoreRecord(ctx context.Context, record *LogRecord) error

	// Migrate runs any necessary schema migrations for the storage backend.
	Migrate(ctx context.Context) error
}

// EntryRecordRetriever handles querying EntryRecord metadata from a storage backend.
type EntryRecordRetriever interface {
	// GetRecord retrieves a single EntryRecord by its request ID.
	GetRecord(ctx context.Context, id uuid.UUID) (*LogRecord, error)

	// NewListRequestsBuilder creates a new builder for listing entry records with filters.
	NewListRequestsBuilder() ListRequestBuilder

	// ListRequestsFromCursor resumes a paginated listing from a cursor string.
	ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error)
}
