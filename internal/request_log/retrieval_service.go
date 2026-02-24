package request_log

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type redisLogRetriever struct {
	r         apredis.Client     `json:"-"`
	blob      apblob.Client      `json:"-"`
	cursorKey config.KeyDataType `json:"-"`
}

func (r *redisLogRetriever) GetFullLog(ctx context.Context, id uuid.UUID) (*Entry, error) {
	client := r.r

	entryRecordData, err := client.HGetAll(ctx, redisLogKey(id)).Result()
	if err != nil {
		return nil, errors.Wrap(err, "failed to retrieve entry record")
	}
	if len(entryRecordData) == 0 {
		return nil, ErrNotFound
	}

	// Try to get full log from blob storage
	if r.blob != nil {
		blobKey := id.String() + ".json"
		data, err := r.blob.Get(ctx, blobKey)
		if err == nil {
			var entry Entry
			if err := json.Unmarshal(data, &entry); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal full log entry")
			}
			return &entry, nil
		} else if !errors.Is(err, apblob.ErrBlobNotFound) {
			return nil, errors.Wrap(err, "failed to retrieve full log entry from blob storage")
		}
	}

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

func NewRetrievalService(r apredis.Client, blob apblob.Client, cursorKey config.KeyDataType) LogRetriever {
	return &redisLogRetriever{
		r:         r,
		blob:      blob,
		cursorKey: cursorKey,
	}
}

var _ LogRetriever = (*redisLogRetriever)(nil)
