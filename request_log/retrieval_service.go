package request_log

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
)

type redisLogRetriever struct {
	r         apredis.Client `json:"-"`
	cursorKey config.KeyData `json:"-"`
}

func (r *redisLogRetriever) GetFullLog(ctx context.Context, id uuid.UUID) (*Entry, error) {
	client := r.r
	data, err := client.Get(ctx, redisFullLogKey(id)).Result()

	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		return nil, err
	}

	return &entry, nil
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

func NewRetrievalService(r apredis.Client, cursorKey config.KeyData) LogRetriever {
	return &redisLogRetriever{
		r:         r,
		cursorKey: cursorKey,
	}
}

var _ LogRetriever = (*redisLogRetriever)(nil)
