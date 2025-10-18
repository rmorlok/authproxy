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
	goredis "github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/util"
	"github.com/rmorlok/authproxy/util/pagination"
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
)

var orderByToRedisSearchField = map[RequestOrderByField]string{
	RequestOrderByTimestamp:          fieldTimestamp,
	RequestOrderByType:               fieldType,
	RequestOrderByCorrelationId:      fieldCorrelationId,
	RequestOrderByConnectionId:       fieldConnectionId,
	RequestOrderByConnectorType:      fieldConnectorType,
	RequestOrderByConnectorId:        fieldConnectorId,
	RequestOrderByMethod:             fieldMethod,
	RequestOrderByPath:               fieldPath,
	RequestOrderByResponseStatusCode: fieldResponseStatusCode,
	RequestOrderByConnectorVersion:   fieldConnectorVersion,
}

type ListRequestExecutor interface {
	FetchPage(context.Context) pagination.PageResult[EntryRecord]
	Enumerate(context.Context, func(pagination.PageResult[EntryRecord]) (keepGoing bool, err error)) error
}

type ListRequestBuilder interface {
	ListRequestExecutor
	Limit(int32) ListRequestBuilder
	OrderBy(RequestOrderByField, pagination.OrderBy) ListRequestBuilder

	/*
	 * Filters
	 */

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

type listRequestsFilters struct {
	r               apredis.Client       `json:"-"`
	cursorKey       config.KeyData       `json:"-"`
	LimitVal        int32                `json:"limit"`
	Offset          int32                `json:"offset"`
	OrderByFieldVal *RequestOrderByField `json:"order_by_field"`
	OrderByVal      *pagination.OrderBy  `json:"order_by"`

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

func (l *listRequestsFilters) Limit(limit int32) ListRequestBuilder {
	l.LimitVal = limit
	return l
}

func (l *listRequestsFilters) WithRequestType(requestType RequestType) ListRequestBuilder {
	l.RequestType = util.ToPtr(string(requestType))
	return l
}

func (l *listRequestsFilters) WithCorrelationId(correlationId string) ListRequestBuilder {
	l.CorrelationId = util.ToPtr(correlationId)
	return l
}

func (l *listRequestsFilters) WithConnectionId(u uuid.UUID) ListRequestBuilder {
	l.ConnectionId = util.ToPtr(u)
	return l
}

// WithConnectorType sets the filter to match a specific connector type. The connector type
// may include wildcards * and %.
func (l *listRequestsFilters) WithConnectorType(t string) ListRequestBuilder {
	l.ConnectorType = util.ToPtr(t)
	return l
}

func (l *listRequestsFilters) WithConnectorId(u uuid.UUID) ListRequestBuilder {
	l.ConnectorId = util.ToPtr(u)
	return l
}

func (l *listRequestsFilters) WithConnectorVersion(v uint64) ListRequestBuilder {
	l.ConnectorVersion = util.ToPtr(v)
	return l
}

func (l *listRequestsFilters) WithMethod(method string) ListRequestBuilder {
	l.Method = util.ToPtr(method)
	return l
}

func (l *listRequestsFilters) WithStatusCode(s int) ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{s, s}
	return l
}

func (l *listRequestsFilters) WithStatusCodeRangeInclusive(start, end int) ListRequestBuilder {
	l.StatusCodeRangeInclusive = []int{start, end}
	return l
}

// WithParsedStatusCodeRange parses the range string and sets the StatusCodeRangeInclusive field. This method will parse
// ranges of the form:
//
//   - 100-200
//   - 100
//
// where a single value is just treated as an exact match. Ranges will be interpreted as includes. If the string is
// empty, if the numbers cannot be parsed, if other characters are included in the string, it will return an error.
//
// The existing filter is modified with the new range and the method returns the same builder.
//
// If this method returns an error, it will be an HTTP status error that can be directly communicated to the API.
func (l *listRequestsFilters) WithParsedStatusCodeRange(r string) (ListRequestBuilder, error) {
	if r == "" {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid status code range; no value specified").
			WithInternalErr(errors.New("no value specified for status code range")).
			Build()
	}

	parts := strings.Split(r, "-")

	if len(parts) > 2 {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; more than one dash", r).
			WithInternalErr(errors.New("more than one dash in status code range")).
			Build()
	}

	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; cannot parse value as integer", r).
			WithInternalErr(err).
			Build()
	}

	if len(parts) == 1 {
		return l.WithStatusCode(start), nil
	}

	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid status code range format: '%s'; cannot parse value as integer", r).
			WithInternalErr(err).
			Build()
	}

	return l.WithStatusCodeRangeInclusive(start, end), nil
}

// WithPath sets the filter to match a specific path. The path can include wildcards * and %.
func (l *listRequestsFilters) WithPath(path string) ListRequestBuilder {
	l.Path = util.ToPtr(path)
	return l
}

// WithPathRegex sets the filter to match a specific path regex. The path regex can include wildcards * and %.
//
// The existing filter is modified with the new path regex and the method returns the same builder.
//
// This method validates the regex at set tite to ensure it is valid. If this method returns an error, it will be an
// HTTP status error that can be directly communicated to the API.
func (l *listRequestsFilters) WithPathRegex(r string) (ListRequestBuilder, error) {
	_, err := regexp.Compile(r)
	if err != nil {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid path regex format: '%s'; cannot compile regex", r).
			WithInternalErr(err).
			Build()
	}

	l.PathRegex = util.ToPtr(r)
	return l, nil
}

func (l *listRequestsFilters) WithTimestampRange(start, end time.Time) ListRequestBuilder {
	l.TimestampRange = []time.Time{start, end}
	return l
}

// WithParsedTimestampRange parses the range string and sets the TimestampRange field. This method will parse
// ranges of the form “2025-10-18T10:23:36Z-2025-10-19T23:59:59Z"
//
// The existing filter is modified with the new range and the method returns the same builder.
//
// If this method returns an error, it will be an HTTP status error that can be directly communicated to the API.
func (l *listRequestsFilters) WithParsedTimestampRange(r string) (ListRequestBuilder, error) {
	if r == "" {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid timestamp range; no value specified").
			WithInternalErr(errors.New("no value specified for timestamp range")).
			Build()
	}

	if strings.Index(r, "-") == -1 {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format: '%s'; no range separator found", r).
			WithInternalErr(errors.New("no range separator in timestamp range")).
			Build()
	}

	if strings.Count(r, "-") != 5 {
		return nil, api_common.
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
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format; invalid start timestamp format: '%s'; must be RFC3339", startStr).
			WithInternalErr(err).
			Build()
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, api_common.
			NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsgf("invalid timestamp range format; invalid end timestamp format: '%s'; must be RFC3339", endStr).
			WithInternalErr(err).
			Build()
	}

	return l.WithTimestampRange(start, end), nil
}

func (l *listRequestsFilters) OrderBy(field RequestOrderByField, by pagination.OrderBy) ListRequestBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listRequestsFilters) FromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	r := l.r
	cursorKey := l.cursorKey

	parsed, err := pagination.ParseCursor[listRequestsFilters](ctx, l.cursorKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.r = r
	l.cursorKey = cursorKey

	return l, nil
}

func (l *listRequestsFilters) apply() (query string, options *goredis.FTSearchOptions) {
	clauses := []string{}
	options = &goredis.FTSearchOptions{}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	options.Limit = int(l.LimitVal + 1) // One additional record as a marker of if there are more
	options.LimitOffset = int(l.Offset)

	sortBy := goredis.FTSearchSortBy{
		FieldName: fieldTimestamp,
		Desc:      true,
	}

	if l.OrderByFieldVal != nil {
		sortBy.FieldName = orderByToRedisSearchField[*l.OrderByFieldVal]
	}

	if l.OrderByVal != nil {
		sortBy.Desc = *l.OrderByVal == pagination.OrderByDesc
		sortBy.Asc = *l.OrderByVal == pagination.OrderByAsc
	}

	options.SortBy = []goredis.FTSearchSortBy{sortBy}

	if l.RequestType != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldType, apredis.EscapeRedisSearchString(*l.RequestType)))
	}

	if l.CorrelationId != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldCorrelationId, apredis.EscapeRedisSearchStringAllowWildcards(*l.RequestType)))
	}

	if l.ConnectionId != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldConnectionId, apredis.EscapeRedisSearchString(l.ConnectionId.String())))
	}

	if l.ConnectorType != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldConnectorType, apredis.EscapeRedisSearchStringAllowWildcards(*l.ConnectorType)))
	}

	if l.ConnectorId != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldConnectorId, apredis.EscapeRedisSearchString(l.ConnectorId.String())))
	}

	if l.ConnectorVersion != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:%d", fieldConnectorVersion, *l.ConnectorVersion))
	}

	if l.Method != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldMethod, apredis.EscapeRedisSearchString(*l.Method)))
	}

	if len(l.StatusCodeRangeInclusive) == 2 {
		clauses = append(clauses, fmt.Sprintf("@%s:[%d %d]", fieldResponseStatusCode, l.StatusCodeRangeInclusive[0], l.StatusCodeRangeInclusive[1]))
	}

	if len(l.TimestampRange) == 2 {
		clauses = append(clauses, fmt.Sprintf("@%s:[%d %d]", fieldTimestamp, l.TimestampRange[0].UnixMilli(), l.TimestampRange[1].UnixMilli()))
	}

	if l.Path != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:(%s)", fieldPath, apredis.EscapeRedisSearchStringAllowWildcards(*l.RequestType)))
	}

	if l.PathRegex != nil {
		clauses = append(clauses, fmt.Sprintf("@%s:/%s/", fieldPath, l.PathRegex))
	}

	if len(clauses) == 0 {
		clauses = append(clauses, "*")
	}

	return strings.Join(clauses, " "), options
}

func (l *listRequestsFilters) validate() error {
	if l.OrderByFieldVal != nil {
		field := orderByToRedisSearchField[*l.OrderByFieldVal]
		if field == "" {
			msg := fmt.Sprintf("invalid order by field '%s'; possible values %s", *l.OrderByFieldVal, util.StringsJoin(util.GetKeys(orderByToRedisSearchField), ", "))
			return api_common.NewHttpStatusErrorBuilder().
				WithStatusBadRequest().
				WithResponseMsg(msg).
				WithInternalErr(errors.New("invalid order by field")).
				BuildStatusError()
		}
	}

	return nil
}

func (l *listRequestsFilters) fetchPage(ctx context.Context) pagination.PageResult[EntryRecord] {
	var err error
	entries := make([]EntryRecord, 0)

	if err = l.validate(); err != nil {
		return pagination.PageResult[EntryRecord]{Error: err}
	}

	client := l.r
	query, options := l.apply()

	res, err := client.FTSearchWithArgs(ctx, RequestLogRedisIndexName, query, options).Result()

	if err != nil {
		return pagination.PageResult[EntryRecord]{Error: err}
	}

	for _, doc := range res.Docs {
		er, err := EntryRecordFromRedisFields(doc.Fields)
		if err != nil {
			return pagination.PageResult[EntryRecord]{Error: err}
		}
		entries = append(entries, *er)
	}

	l.Offset = l.Offset + int32(len(entries)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(entries)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.cursorKey, l)
		if err != nil {
			return pagination.PageResult[EntryRecord]{Error: err}
		}
	}

	return pagination.PageResult[EntryRecord]{
		HasMore: hasMore,
		Results: entries[:util.MinInt32(l.LimitVal, int32(len(entries)))],
		Cursor:  cursor,
	}
}

func (l *listRequestsFilters) FetchPage(ctx context.Context) pagination.PageResult[EntryRecord] {
	return l.fetchPage(ctx)
}

func (l *listRequestsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[EntryRecord]) (keepGoing bool, err error)) error {
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

var _ ListRequestExecutor = (*listRequestsFilters)(nil)
var _ ListRequestBuilder = (*listRequestsFilters)(nil)
