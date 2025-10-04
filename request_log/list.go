package request_log

import (
	"context"
	"strings"

	goredis "github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/util"
	"github.com/rmorlok/authproxy/util/pagination"
)

type RequestOrderByField string

const (
	RequestOrderByCreatedAt RequestOrderByField = "created_at"
)

type ListRequestExecutor interface {
	FetchPage(context.Context) pagination.PageResult[EntryRecord]
	Enumerate(context.Context, func(pagination.PageResult[EntryRecord]) (keepGoing bool, err error)) error
}

type ListRequestBuilder interface {
	ListRequestExecutor
	Limit(int32) ListRequestBuilder
	OrderBy(RequestOrderByField, pagination.OrderBy) ListRequestBuilder
}

type listRequestsFilters struct {
	r               apredis.Client       `json:"-"`
	cursorKey       config.KeyData       `json:"-"`
	LimitVal        int32                `json:"limit"`
	Offset          int32                `json:"offset"`
	OrderByFieldVal *RequestOrderByField `json:"order_by_field"`
	OrderByVal      *pagination.OrderBy  `json:"order_by"`
}

func (l *listRequestsFilters) Limit(limit int32) ListRequestBuilder {
	l.LimitVal = limit
	return l
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

	if l.OrderByVal != nil {
		sortBy.Desc = *l.OrderByVal == pagination.OrderByDesc
		sortBy.Asc = *l.OrderByVal == pagination.OrderByAsc
	}

	if l.OrderByFieldVal != nil {
		sortBy.FieldName = string(*l.OrderByFieldVal)
	}

	if len(clauses) == 0 {
		clauses = append(clauses, "*")
	}

	return strings.Join(clauses, " "), options
}

func (l *listRequestsFilters) fetchPage(ctx context.Context) pagination.PageResult[EntryRecord] {
	var err error
	var entries []EntryRecord

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
