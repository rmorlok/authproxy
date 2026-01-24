package mock

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/request_log"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type MockListRequestBuilderExecutor struct {
	FromCursorError error
	ReturnResults   pagination.PageResult[*request_log.EntryRecord]
	CursorVal       string
	LimitVal        int32
	OffsetVal       int32
	OrderByFieldVal *request_log.RequestOrderByField
	OrderByVal      *pagination.OrderBy

	Namespace                *string     `json:"namespace,omitempty"`
	Namespaces               []string    `json:"namespaces,omitempty"`
	RequestType              *string     `json:"request_type,omitempty"`
	CorrelationId            *string     `json:"correlation_id,omitempty"`
	ConnectionId             *uuid.UUID  `json:"connection_id,omitempty"`
	ConnectorType            *string     `json:"connector_type,omitempty"`
	ConnectorId              *uuid.UUID  `json:"connector_id,omitempty"`
	ConnectorVersion         *uint64     `json:"connector_version,omitempty"`
	Method                   *string     `json:"method,omitempty"`
	StatusCodeRangeInclusive []int       `json:"status_code_range,omitempty"`
	TimestampRange           []time.Time `json:"timestamp_range,omitempty"`
	Path                     *string     `json:"path,omitempty"`
	PathRegex                *string     `json:"path_regex,omitempty"`
}

func (l *MockListRequestBuilderExecutor) WithNamespaceMatcher(matcher string) request_log.ListRequestBuilder {
	l.Namespace = util.ToPtr(matcher)
	return l
}

func (l *MockListRequestBuilderExecutor) WithNamespaceMatchers(matchers []string) request_log.ListRequestBuilder {
	l.Namespaces = matchers
	return l
}

func (l *MockListRequestBuilderExecutor) WithRequestType(requestType request_log.RequestType) request_log.ListRequestBuilder {
	l.RequestType = util.ToPtr(string(requestType))
	return l
}

func (l *MockListRequestBuilderExecutor) WithCorrelationId(correlationId string) request_log.ListRequestBuilder {
	l.CorrelationId = util.ToPtr(correlationId)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectionId(u uuid.UUID) request_log.ListRequestBuilder {
	l.ConnectionId = util.ToPtr(u)
	return l
}

// WithConnectorType sets the filter to match a specific connector type. The connector type
// may include wildcards * and %.
func (l *MockListRequestBuilderExecutor) WithConnectorType(t string) request_log.ListRequestBuilder {
	l.ConnectorType = util.ToPtr(t)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectorId(u uuid.UUID) request_log.ListRequestBuilder {
	l.ConnectorId = util.ToPtr(u)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectorVersion(v uint64) request_log.ListRequestBuilder {
	l.ConnectorVersion = util.ToPtr(v)
	return l
}

func (l *MockListRequestBuilderExecutor) WithMethod(method string) request_log.ListRequestBuilder {
	l.Method = util.ToPtr(method)
	return l
}

func (l *MockListRequestBuilderExecutor) WithStatusCode(s int) request_log.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{s, s}
	return l
}

func (l *MockListRequestBuilderExecutor) WithStatusCodeRangeInclusive(start, end int) request_log.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) WithParsedStatusCodeRange(r string) (request_log.ListRequestBuilder, error) {
	// No op
	return l, nil
}

// WithPath sets the filter to match a specific path. The path can include wildcards * and %.
func (l *MockListRequestBuilderExecutor) WithPath(path string) request_log.ListRequestBuilder {
	l.Path = util.ToPtr(path)
	return l
}

func (l *MockListRequestBuilderExecutor) WithPathRegex(r string) (request_log.ListRequestBuilder, error) {
	l.PathRegex = util.ToPtr(r)
	return l, nil
}

func (l *MockListRequestBuilderExecutor) WithTimestampRange(start, end time.Time) request_log.ListRequestBuilder {
	l.TimestampRange = []time.Time{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) WithParsedTimestampRange(r string) (request_log.ListRequestBuilder, error) {
	return l, nil
}

func (l *MockListRequestBuilderExecutor) Limit(limit int32) request_log.ListRequestBuilder {
	l.LimitVal = limit
	return l
}

func (l *MockListRequestBuilderExecutor) OrderBy(field request_log.RequestOrderByField, by pagination.OrderBy) request_log.ListRequestBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *MockListRequestBuilderExecutor) FromCursor(_ context.Context, cursor string) (request_log.ListRequestExecutor, error) {
	l.CursorVal = cursor
	return l, l.FromCursorError
}

func (l *MockListRequestBuilderExecutor) FetchPage(ctx context.Context) pagination.PageResult[*request_log.EntryRecord] {
	return l.ReturnResults
}

func (l *MockListRequestBuilderExecutor) Enumerate(ctx context.Context, callback func(pagination.PageResult[*request_log.EntryRecord]) (keepGoing bool, err error)) error {
	var err error
	keepGoing := true
	hasMore := true

	for err == nil && hasMore && keepGoing {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

var _ request_log.ListRequestExecutor = &MockListRequestBuilderExecutor{}
var _ request_log.ListRequestBuilder = &MockListRequestBuilderExecutor{}
