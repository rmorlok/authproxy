package mock

import (
	"context"

	"github.com/rmorlok/authproxy/request_log"
	"github.com/rmorlok/authproxy/util/pagination"
)

type MockListRequestBuilderExecutor struct {
	FromCursorError error
	ReturnResults   pagination.PageResult[request_log.EntryRecord]
	CursorVal       string
	LimitVal        int32
	OffsetVal       int32
	OrderByFieldVal *request_log.RequestOrderByField
	OrderByVal      *pagination.OrderBy
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

func (l *MockListRequestBuilderExecutor) FetchPage(ctx context.Context) pagination.PageResult[request_log.EntryRecord] {
	return l.ReturnResults
}

func (l *MockListRequestBuilderExecutor) Enumerate(ctx context.Context, callback func(pagination.PageResult[request_log.EntryRecord]) (keepGoing bool, err error)) error {
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
