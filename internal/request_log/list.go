package request_log

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/api_common"
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
	FetchPage(ctx context.Context) pagination.PageResult[*EntryRecord]
	Enumerate(ctx context.Context, callback func(pagination.PageResult[*EntryRecord]) (keepGoing bool, err error)) error
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
	WithRequestType(requestType RequestType) ListRequestBuilder
	WithCorrelationId(correlationId string) ListRequestBuilder
	WithConnectionId(u uuid.UUID) ListRequestBuilder
	WithConnectorType(t string) ListRequestBuilder
	WithConnectorId(u uuid.UUID) ListRequestBuilder
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
	ConnectionId             *uuid.UUID        `json:"connection_id,omitempty"`
	ConnectorType            *string           `json:"connector_type,omitempty"`
	ConnectorId              *uuid.UUID        `json:"connector_id,omitempty"`
	ConnectorVersion         *uint64           `json:"connector_version,omitempty"`
	Method                   *string           `json:"method,omitempty"`
	StatusCodeRangeInclusive []int             `json:"status_code_range,omitempty"`
	TimestampRange           []time.Time       `json:"timestamp_range,omitempty"`
	Path                     *string           `json:"path,omitempty"`
	PathRegex                *string           `json:"path_regex,omitempty"`
	NamespaceMatchers        []string          `json:"namespace_matchers,omitempty"`
	Errors                   *multierror.Error `json:"-"`
}

func (l *ListFilters) AddError(e error) {
	l.Errors = multierror.Append(l.Errors, e)
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

func (l *ListFilters) SetRequestType(requestType RequestType) {
	l.RequestType = util.ToPtr(string(requestType))
}

func (l *ListFilters) SetCorrelationId(correlationId string) {
	l.CorrelationId = util.ToPtr(correlationId)
}

func (l *ListFilters) SetConnectionId(u uuid.UUID) {
	l.ConnectionId = util.ToPtr(u)
}

func (l *ListFilters) SetConnectorType(t string) {
	l.ConnectorType = util.ToPtr(t)
}

func (l *ListFilters) SetConnectorId(u uuid.UUID) {
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

// ParseStatusCodeRange parses the range string and returns start, end values.
func ParseStatusCodeRange(r string) (int, int, error) {
	if r == "" {
		return 0, 0, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid status code range; no value specified").
			WithInternalErr(errors.New("no value specified for status code range")).
			Build()
	}

	parts := strings.Split(r, "-")

	if len(parts) > 2 {
		return 0, 0, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; more than one dash", r).
			WithInternalErr(errors.New("more than one dash in status code range")).
			Build()
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; cannot parse value as integer", r).
			WithInternalErr(err).
			Build()
	}

	if len(parts) == 1 {
		return start, start, nil
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; cannot parse value as integer", r).
			WithInternalErr(err).
			Build()
	}

	return start, end, nil
}

// ParseTimestampRange parses the range string and returns start, end values.
func ParseTimestampRange(r string) (time.Time, time.Time, error) {
	if r == "" {
		return time.Time{}, time.Time{}, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid timestamp range; no value specified").
			WithInternalErr(errors.New("no value specified for timestamp range")).
			Build()
	}

	if strings.Index(r, "-") == -1 {
		return time.Time{}, time.Time{}, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format: '%s'; no range separator found", r).
			WithInternalErr(errors.New("no range separator in timestamp range")).
			Build()
	}

	if strings.Count(r, "-") != 5 {
		return time.Time{}, time.Time{}, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format: '%s'; must be YYYY-MM-DDTHH:MM:SSZ-YYYY-MM-DDTHH:MM:SSZ", r).
			WithInternalErr(errors.New("invalid timestamp range format")).
			Build()
	}

	// Find the third dash which separates the two timestamps
	firstDash := strings.Index(r, "-")
	secondDash := strings.Index(r[firstDash+1:], "-") + firstDash + 1
	thirdDashIndex := strings.Index(r[secondDash+1:], "-") + secondDash + 1

	startStr := r[:thirdDashIndex]
	endStr := r[thirdDashIndex+1:]

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format; invalid start timestamp format: '%s'; must be RFC3339", startStr).
			WithInternalErr(err).
			Build()
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format; invalid end timestamp format: '%s'; must be RFC3339", endStr).
			WithInternalErr(err).
			Build()
	}

	return start, end, nil
}
