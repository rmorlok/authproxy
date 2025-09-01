package request_log

import (
	"context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/redis"
)

type redisLogRetriever struct {
	r         redis.R        `json:"-"`
	cursorKey config.KeyData `json:"-"`
}

func (r *redisLogRetriever) GetFullLog(id uuid.UUID) (*Entry, error) {
	return nil, nil
}

func (r *redisLogRetriever) NewListRequestsBuilder() ListRequestBuilder {
	return &listRequestsFilters{
		r:         r.r,
		cursorKey: r.cursorKey,
		LimitVal:  100,
	}
}

func (r *redisLogRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	b := &listRequestsFilters{
		r:         r.r,
		cursorKey: r.cursorKey,
		LimitVal:  100,
	}

	return b.FromCursor(ctx, cursor)
}

func NewRetrievalService(r redis.R, cursorKey config.KeyData) LogRetriever {
	return &redisLogRetriever{
		r:         r,
		cursorKey: cursorKey,
	}
}

var _ LogRetriever = (*redisLogRetriever)(nil)
