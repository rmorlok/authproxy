package request_log

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Logger is an interface for logging requests. This is used as a middleware for http.Client.
type Logger interface {
	RoundTrip(req *http.Request) (*http.Response, error)
}

// LogRetriever is an interface for retrieving logs. Used by the API to retrieve logs.
type LogRetriever interface {
	GetFullLog(id uuid.UUID) (*Entry, error)
	NewListRequestsBuilder() ListRequestBuilder
	ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error)
}
