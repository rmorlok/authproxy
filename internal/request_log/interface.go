package request_log

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
)

// LogRetriever is an interface for retrieving logs. Used by the API to retrieve logs.
//
//go:generate mockgen -source=./interface.go -destination=./mock/service.go -package=mock
type LogRetriever interface {
	GetFullLog(ctx context.Context, id apid.ID) (*FullLog, error)
	NewListRequestsBuilder() ListRequestBuilder
	ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error)
}
