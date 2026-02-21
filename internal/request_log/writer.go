package request_log

import "context"

// LogWriter handles persisting LogRecord metadata to a storage backend.
type LogWriter interface {
	// Write writes a single record to the store
	Write(ctx context.Context, record *LogRecord) error

	// WriteMulti writes a multiple records to the store
	WriteMulti(ctx context.Context, records []*LogRecord) error
}
