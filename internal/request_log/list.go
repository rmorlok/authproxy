package request_log

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/httpf"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type RequestOrderByField string

const (
	RequestOrderByTimestamp          RequestOrderByField = "timestamp"
	RequestOrderByType               RequestOrderByField = "type"
	RequestOrderByCorrelationId      RequestOrderByField = "correlation_id"
	RequestOrderByConnectionId       RequestOrderByField = "connection_id"
	RequestOrderByConnectorType      RequestOrderByField = "connector_type"
	RequestOrderByConnectorId        RequestOrderByField = "connector_id"
	RequestOrderByMethod             RequestOrderByField = "method"
	RequestOrderByPath               RequestOrderByField = "path"
	RequestOrderByResponseStatusCode RequestOrderByField = "response_status_code"
	RequestOrderByConnectorVersion   RequestOrderByField = "connector_version"
	RequestOrderByNamespace          RequestOrderByField = "namespace"
)

var validOrderByFields = map[RequestOrderByField]bool{
	RequestOrderByTimestamp:          true,
	RequestOrderByType:               true,
	RequestOrderByCorrelationId:      true,
	RequestOrderByConnectionId:       true,
	RequestOrderByConnectorType:      true,
	RequestOrderByConnectorId:        true,
	RequestOrderByMethod:             true,
	RequestOrderByPath:               true,
	RequestOrderByResponseStatusCode: true,
	RequestOrderByConnectorVersion:   true,
	RequestOrderByNamespace:          true,
}

func IsValidOrderByField(field RequestOrderByField) bool {
	return validOrderByFields[field]
}

type ListRequestExecutor interface {
	FetchPage(ctx context.Context) pagination.PageResult[*LogRecord]
	Enumerate(ctx context.Context, callback func(pagination.PageResult[*LogRecord]) (keepGoing bool, err error)) error
}

type ListRequestBuilder interface {
	ListRequestExecutor
	Limit(int32) ListRequestBuilder
	OrderBy(RequestOrderByField, pagination.OrderBy) ListRequestBuilder

	/*
	 * Filters
	 */

	WithNamespaceMatcher(matcher string) ListRequestBuilder
	WithNamespaceMatchers(matchers []string) ListRequestBuilder
	WithRequestType(requestType httpf.RequestType) ListRequestBuilder
	WithCorrelationId(correlationId string) ListRequestBuilder
	WithConnectionId(u apid.ID) ListRequestBuilder
	WithConnectorType(t string) ListRequestBuilder
	WithConnectorId(u apid.ID) ListRequestBuilder
	WithConnectorVersion(v uint64) ListRequestBuilder
	WithMethod(method string) ListRequestBuilder
	WithStatusCode(s int) ListRequestBuilder
	WithStatusCodeRangeInclusive(start, end int) ListRequestBuilder
	WithParsedStatusCodeRange(r string) (ListRequestBuilder, error)
	WithPath(path string) ListRequestBuilder
	WithPathRegex(r string) (ListRequestBuilder, error)
	WithTimestampRange(start, end time.Time) ListRequestBuilder
	WithParsedTimestampRange(r string) (ListRequestBuilder, error)
}

// ListFilters holds the filter, pagination, and ordering data for list requests.
// This struct is provider-agnostic and is embedded by provider-specific list builders.
type ListFilters struct {
	LimitVal        int32                `json:"limit"`
	Offset          int32                `json:"offset"`
	OrderByFieldVal *RequestOrderByField `json:"order_by_field"`
	OrderByVal      *pagination.OrderBy  `json:"order_by"`

	RequestType              *string           `json:"request_type,omitempty"`
	CorrelationId            *string           `json:"correlation_id,omitempty"`
	ConnectionId             *apid.ID        `json:"connection_id,omitempty"`
	ConnectorType            *string           `json:"connector_type,omitempty"`
	ConnectorId              *apid.ID        `json:"connector_id,omitempty"`
	ConnectorVersion         *uint64           `json:"connector_version,omitempty"`
	Method                   *string           `json:"method,omitempty"`
	StatusCodeRangeInclusive []int             `json:"status_code_range,omitempty"`
	TimestampRange           []time.Time       `json:"timestamp_range,omitempty"`
	Path                     *string           `json:"path,omitempty"`
	PathRegex                *string           `json:"path_regex,omitempty"`
	NamespaceMatchers        []string          `json:"namespace_matchers,omitempty"`
	Errors                   *multierror.Error `json:"-"`
}

func (l *ListFilters) Validate() error {
	if l.OrderByFieldVal != nil {
		if !IsValidOrderByField(*l.OrderByFieldVal) {
			msg := fmt.Sprintf("invalid order by field '%s'", *l.OrderByFieldVal)
			return api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsg(msg).
				WithInternalErr(errors.New("invalid order by field")).
				BuildStatusError()
		}
	}

	return nil
}

func (l *ListFilters) AddError(e error) {
	l.Errors = multierror.Append(l.Errors, e)
}

func (l *ListFilters) SetLimit(limit int32) {
	l.LimitVal = limit
}

func (l *ListFilters) SetOrderBy(field RequestOrderByField, by pagination.OrderBy) {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
}

func (l *ListFilters) SetNamespaceMatcher(matcher string) error {
	if err := aschema.ValidateNamespaceMatcher(matcher); err != nil {
		return err
	}
	l.NamespaceMatchers = []string{matcher}
	return nil
}

func (l *ListFilters) SetNamespaceMatchers(matchers []string) error {
	for _, matcher := range matchers {
		if err := aschema.ValidateNamespaceMatcher(matcher); err != nil {
			return err
		}
	}
	l.NamespaceMatchers = matchers
	return nil
}

func (l *ListFilters) SetRequestType(requestType httpf.RequestType) {
	l.RequestType = util.ToPtr(string(requestType))
}

func (l *ListFilters) SetCorrelationId(correlationId string) {
	l.CorrelationId = util.ToPtr(correlationId)
}

func (l *ListFilters) SetConnectionId(u apid.ID) {
	l.ConnectionId = util.ToPtr(u)
}

func (l *ListFilters) SetConnectorType(t string) {
	l.ConnectorType = util.ToPtr(t)
}

func (l *ListFilters) SetConnectorId(u apid.ID) {
	l.ConnectorId = util.ToPtr(u)
}

func (l *ListFilters) SetConnectorVersion(v uint64) {
	l.ConnectorVersion = util.ToPtr(v)
}

func (l *ListFilters) SetMethod(method string) {
	l.Method = util.ToPtr(method)
}

func (l *ListFilters) SetStatusCode(s int) {
	l.StatusCodeRangeInclusive = []int{s, s}
}

func (l *ListFilters) SetStatusCodeRangeInclusive(start, end int) {
	l.StatusCodeRangeInclusive = []int{start, end}
}

func (l *ListFilters) SetPath(path string) {
	l.Path = util.ToPtr(path)
}

func (l *ListFilters) SetPathRegex(r string) error {
	_, err := regexp.Compile(r)
	if err != nil {
		return api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid path regex format: '%s'; cannot compile regex", r).
			WithInternalErr(err).
			Build()
	}

	l.PathRegex = util.ToPtr(r)
	return nil
}

func (l *ListFilters) SetTimestampRange(start, end time.Time) {
	l.TimestampRange = []time.Time{start, end}
}
