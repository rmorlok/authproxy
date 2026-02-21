package request_log

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type redisLogRetriever struct {
	r                apredis.Client           `json:"-"`
	cursorKey        config.KeyDataType       `json:"-"`
	recordRetriever  EntryRecordRetriever
}

func (r *redisLogRetriever) GetFullLog(ctx context.Context, id uuid.UUID) (*Entry, error) {
	client := r.r
	pipeline := client.Pipeline()

	entryRecordCmd := pipeline.HGetAll(ctx, redisLogKey(id))
	fullResultCmd := pipeline.Get(ctx, redisFullLogKey(id))

	_, err := pipeline.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	entryRecordData, err := entryRecordCmd.Result()
	if errors.Is(err, redis.Nil) || (err == nil && len(entryRecordData) == 0) {
		// Try the record retriever as fallback
		if r.recordRetriever != nil {
			er, rerr := r.recordRetriever.GetRecord(ctx, id)
			if rerr != nil {
				return nil, ErrNotFound
			}
			entry := NewEntryFromRecord(er)
			entry.Full = false
			return entry, nil
		}
		return nil, ErrNotFound
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve entry record")
	}

	data, err := fullResultCmd.Result()

	if err == nil {
		// Full data available
		var entry Entry
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			return nil, errors.Wrap(err, "failed to unmarshal full log entry")
		}

		return &entry, nil
	} else if errors.Is(err, redis.Nil) {
		// Full data not available, extract what we can from entry record
		er, err := EntryRecordFromRedisFields(entryRecordData)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse entry record from redis data")
		}

		entry := NewEntryFromRecord(er)
		// Even if we previously think we have stored data, at this point we know we don't have the full request/response
		entry.Full = false

		return entry, nil
	}

	return nil, errors.Wrap(err, "failed to retrieve full log entry")
}

func (r *redisLogRetriever) NewListRequestsBuilder() ListRequestBuilder {
	if r.recordRetriever != nil {
		return r.recordRetriever.NewListRequestsBuilder()
	}
	return &redisListRequestsBuilder{
		ListFilters: ListFilters{LimitVal: 100},
		r:           r.r,
		cursorKey:   r.cursorKey,
	}
}

func (r *redisLogRetriever) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	if r.recordRetriever != nil {
		return r.recordRetriever.ListRequestsFromCursor(ctx, cursor)
	}
	b := &redisListRequestsBuilder{
		ListFilters: ListFilters{LimitVal: 100},
		r:           r.r,
		cursorKey:   r.cursorKey,
	}

	return b.FromCursor(ctx, cursor)
}

// NewRetrievalService creates a LogRetriever backed by Redis.
// Deprecated: Use NewRetrievalServiceWithProvider instead.
func NewRetrievalService(r apredis.Client, cursorKey config.KeyDataType) LogRetriever {
	return &redisLogRetriever{
		r:         r,
		cursorKey: cursorKey,
	}
}

// NewRetrievalServiceWithProvider creates a LogRetriever with a pluggable EntryRecordRetriever
// for metadata queries. Redis is still used for full request JSON retrieval.
func NewRetrievalServiceWithProvider(r apredis.Client, retriever EntryRecordRetriever) LogRetriever {
	return &redisLogRetriever{
		r:               r,
		recordRetriever: retriever,
	}
}

var _ LogRetriever = (*redisLogRetriever)(nil)
