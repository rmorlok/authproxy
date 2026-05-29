package app_metrics

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
	QueryRequestEventMetrics(ctx context.Context, queries []RequestEventMetricsQuery) ([]RequestEventMetricSeries, error)
	QueryResourceMetrics(ctx context.Context, queries []ResourceMetricsQuery) ([]ResourceMetricSeries, error)
}
