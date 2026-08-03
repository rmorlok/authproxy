package mock

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type MockListRequestBuilderExecutor struct {
	FromCursorError error
	ReturnResults   pagination.PageResult[*app_metrics.LogRecord]
	CursorVal       string
	LimitVal        int32
	OffsetVal       int32
	OrderByFieldVal *app_metrics.RequestOrderByField
	OrderByVal      *pagination.OrderBy

	Namespace                *string     `json:"namespace,omitempty"`
	Namespaces               []string    `json:"namespaces,omitempty"`
	RequestType              *string     `json:"requestType,omitempty"`
	CorrelationId            *string     `json:"correlationId,omitempty"`
	ConnectionId             *apid.ID    `json:"connectionId,omitempty"`
	ConnectorType            *string     `json:"connectorType,omitempty"`
	ConnectorId              *apid.ID    `json:"connectorId,omitempty"`
	ConnectorVersion         *uint64     `json:"connectorVersion,omitempty"`
	Method                   *string     `json:"method,omitempty"`
	StatusCodeRangeInclusive []int       `json:"statusCodeRange,omitempty"`
	TimestampRange           []time.Time `json:"timestampRange,omitempty"`
	Path                     *string     `json:"path,omitempty"`
	PathRegex                *string     `json:"pathRegex,omitempty"`
	LabelSelector            *string     `json:"labelSelector,omitempty"`
	ResponseSource           *string     `json:"responseSource,omitempty"`
	RateLimitId              *apid.ID    `json:"rateLimitId,omitempty"`
}

func (l *MockListRequestBuilderExecutor) ForNamespaceMatcher(matcher string) app_metrics.ListRequestBuilder {
	l.Namespace = util.ToPtr(matcher)
	return l
}

func (l *MockListRequestBuilderExecutor) ForNamespaceMatchers(matchers []string) app_metrics.ListRequestBuilder {
	l.Namespaces = matchers
	return l
}

func (l *MockListRequestBuilderExecutor) ForRequestType(requestType httpf.RequestType) app_metrics.ListRequestBuilder {
	l.RequestType = util.ToPtr(string(requestType))
	return l
}

func (l *MockListRequestBuilderExecutor) ForCorrelationId(correlationId string) app_metrics.ListRequestBuilder {
	l.CorrelationId = util.ToPtr(correlationId)
	return l
}

func (l *MockListRequestBuilderExecutor) ForConnectionId(u apid.ID) app_metrics.ListRequestBuilder {
	l.ConnectionId = util.ToPtr(u)
	return l
}

// ForConnectorType sets the filter to match a specific connector type. The connector type
// may include wildcards * and %.
func (l *MockListRequestBuilderExecutor) ForConnectorType(t string) app_metrics.ListRequestBuilder {
	l.ConnectorType = util.ToPtr(t)
	return l
}

func (l *MockListRequestBuilderExecutor) ForConnectorId(u apid.ID) app_metrics.ListRequestBuilder {
	l.ConnectorId = util.ToPtr(u)
	return l
}

func (l *MockListRequestBuilderExecutor) ForConnectorVersion(v uint64) app_metrics.ListRequestBuilder {
	l.ConnectorVersion = util.ToPtr(v)
	return l
}

func (l *MockListRequestBuilderExecutor) ForMethod(method string) app_metrics.ListRequestBuilder {
	l.Method = util.ToPtr(method)
	return l
}

func (l *MockListRequestBuilderExecutor) ForStatusCode(s int) app_metrics.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{s, s}
	return l
}

func (l *MockListRequestBuilderExecutor) ForStatusCodeRangeInclusive(start, end int) app_metrics.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) ForParsedStatusCodeRange(r string) (app_metrics.ListRequestBuilder, error) {
	start, end, err := util.ParseHTTPStatusCodeRange(r)
	if err != nil {
		return nil, err
	}
	l.StatusCodeRangeInclusive = []int{start, end}
	return l, nil
}

// ForPath sets the filter to match a specific path. The path can include wildcards * and %.
func (l *MockListRequestBuilderExecutor) ForPath(path string) app_metrics.ListRequestBuilder {
	l.Path = util.ToPtr(path)
	return l
}

func (l *MockListRequestBuilderExecutor) ForPathRegex(r string) (app_metrics.ListRequestBuilder, error) {
	l.PathRegex = util.ToPtr(r)
	return l, nil
}

func (l *MockListRequestBuilderExecutor) ForTimestampRange(start, end time.Time) app_metrics.ListRequestBuilder {
	l.TimestampRange = []time.Time{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) ForParsedTimestampRange(r string) (app_metrics.ListRequestBuilder, error) {
	return l, nil
}

func (l *MockListRequestBuilderExecutor) ForLabelSelector(selector string) (app_metrics.ListRequestBuilder, error) {
	l.LabelSelector = util.ToPtr(selector)
	return l, nil
}

func (l *MockListRequestBuilderExecutor) ForResponseSource(s app_metrics.ResponseSource) app_metrics.ListRequestBuilder {
	l.ResponseSource = util.ToPtr(string(s))
	return l
}

func (l *MockListRequestBuilderExecutor) ForRateLimitId(id apid.ID) app_metrics.ListRequestBuilder {
	l.RateLimitId = util.ToPtr(id)
	return l
}

func (l *MockListRequestBuilderExecutor) Limit(limit int32) app_metrics.ListRequestBuilder {
	l.LimitVal = limit
	return l
}

func (l *MockListRequestBuilderExecutor) OrderBy(field app_metrics.RequestOrderByField, by pagination.OrderBy) app_metrics.ListRequestBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *MockListRequestBuilderExecutor) FromCursor(_ context.Context, cursor string) (app_metrics.ListRequestExecutor, error) {
	l.CursorVal = cursor
	return l, l.FromCursorError
}

func (l *MockListRequestBuilderExecutor) FetchPage(ctx context.Context) pagination.PageResult[*app_metrics.LogRecord] {
	return l.ReturnResults
}

func (l *MockListRequestBuilderExecutor) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[*app_metrics.LogRecord]) error {
	var err error
	keepGoing := pagination.Continue
	hasMore := true

	for err == nil && hasMore && bool(keepGoing) {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

var _ app_metrics.ListRequestExecutor = &MockListRequestBuilderExecutor{}
var _ app_metrics.ListRequestBuilder = &MockListRequestBuilderExecutor{}
