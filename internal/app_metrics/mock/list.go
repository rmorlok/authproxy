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
	RequestType              *string     `json:"request_type,omitempty"`
	CorrelationId            *string     `json:"correlation_id,omitempty"`
	ConnectionId             *apid.ID    `json:"connection_id,omitempty"`
	ConnectorType            *string     `json:"connector_type,omitempty"`
	ConnectorId              *apid.ID    `json:"connector_id,omitempty"`
	ConnectorVersion         *uint64     `json:"connector_version,omitempty"`
	Method                   *string     `json:"method,omitempty"`
	StatusCodeRangeInclusive []int       `json:"status_code_range,omitempty"`
	TimestampRange           []time.Time `json:"timestamp_range,omitempty"`
	Path                     *string     `json:"path,omitempty"`
	PathRegex                *string     `json:"path_regex,omitempty"`
	LabelSelector            *string     `json:"label_selector,omitempty"`
	ResponseSource           *string     `json:"response_source,omitempty"`
	RateLimitId              *apid.ID    `json:"rate_limit_id,omitempty"`
}

func (l *MockListRequestBuilderExecutor) WithNamespaceMatcher(matcher string) app_metrics.ListRequestBuilder {
	l.Namespace = util.ToPtr(matcher)
	return l
}

func (l *MockListRequestBuilderExecutor) WithNamespaceMatchers(matchers []string) app_metrics.ListRequestBuilder {
	l.Namespaces = matchers
	return l
}

func (l *MockListRequestBuilderExecutor) WithRequestType(requestType httpf.RequestType) app_metrics.ListRequestBuilder {
	l.RequestType = util.ToPtr(string(requestType))
	return l
}

func (l *MockListRequestBuilderExecutor) WithCorrelationId(correlationId string) app_metrics.ListRequestBuilder {
	l.CorrelationId = util.ToPtr(correlationId)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectionId(u apid.ID) app_metrics.ListRequestBuilder {
	l.ConnectionId = util.ToPtr(u)
	return l
}

// WithConnectorType sets the filter to match a specific connector type. The connector type
// may include wildcards * and %.
func (l *MockListRequestBuilderExecutor) WithConnectorType(t string) app_metrics.ListRequestBuilder {
	l.ConnectorType = util.ToPtr(t)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectorId(u apid.ID) app_metrics.ListRequestBuilder {
	l.ConnectorId = util.ToPtr(u)
	return l
}

func (l *MockListRequestBuilderExecutor) WithConnectorVersion(v uint64) app_metrics.ListRequestBuilder {
	l.ConnectorVersion = util.ToPtr(v)
	return l
}

func (l *MockListRequestBuilderExecutor) WithMethod(method string) app_metrics.ListRequestBuilder {
	l.Method = util.ToPtr(method)
	return l
}

func (l *MockListRequestBuilderExecutor) WithStatusCode(s int) app_metrics.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{s, s}
	return l
}

func (l *MockListRequestBuilderExecutor) WithStatusCodeRangeInclusive(start, end int) app_metrics.ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) WithParsedStatusCodeRange(r string) (app_metrics.ListRequestBuilder, error) {
	// No op
	return l, nil
}

// WithPath sets the filter to match a specific path. The path can include wildcards * and %.
func (l *MockListRequestBuilderExecutor) WithPath(path string) app_metrics.ListRequestBuilder {
	l.Path = util.ToPtr(path)
	return l
}

func (l *MockListRequestBuilderExecutor) WithPathRegex(r string) (app_metrics.ListRequestBuilder, error) {
	l.PathRegex = util.ToPtr(r)
	return l, nil
}

func (l *MockListRequestBuilderExecutor) WithTimestampRange(start, end time.Time) app_metrics.ListRequestBuilder {
	l.TimestampRange = []time.Time{start, end}
	return l
}

func (l *MockListRequestBuilderExecutor) WithParsedTimestampRange(r string) (app_metrics.ListRequestBuilder, error) {
	return l, nil
}

func (l *MockListRequestBuilderExecutor) WithLabelSelector(selector string) (app_metrics.ListRequestBuilder, error) {
	l.LabelSelector = util.ToPtr(selector)
	return l, nil
}

func (l *MockListRequestBuilderExecutor) WithResponseSource(s app_metrics.ResponseSource) app_metrics.ListRequestBuilder {
	l.ResponseSource = util.ToPtr(string(s))
	return l
}

func (l *MockListRequestBuilderExecutor) WithRateLimitId(id apid.ID) app_metrics.ListRequestBuilder {
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
