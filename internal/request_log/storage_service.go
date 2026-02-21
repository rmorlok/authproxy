package request_log

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

type StorageService struct {
	r         apredis.Client
	logger    *slog.Logger
	logWriter LogWriter
}

// NewStorageService that will store the log records and the full request/response.
func NewStorageService(
	cfg *config.HttpLogging,
	logger *slog.Logger,
) *StorageService {
	l := &StorageService{
		r:                     r,
		logWriter:             recordStore,
		logger:                logger,
		requestInfo:           requestInfo,
		expiration:            expiration,
		recordFullRequest:     recordFullRequest,
		fullRequestExpiration: fullRequestExpiration,
		maxFullRequestSize:    maxFullRequestSize,
		maxFullResponseSize:   maxFullResponseSize,
		maxResponseWait:       maxResponseWait,
		transport:             transport,
	}

	// Apply default behavior
	l.persistEntry = l.storeEntry

	return l
}

// NewRedisLogger creates a new Logger backed entirely by Redis.
// Deprecated: Use NewLogger with a Redis EntryRecordStore instead.
func NewRedisLogger(
	r apredis.Client,
	logger *slog.Logger,
	requestInfo RequestInfo,
	expiration time.Duration,
	recordFullRequest bool,
	fullRequestExpiration time.Duration,
	maxFullRequestSize uint64,
	maxFullResponseSize uint64,
	maxResponseWait time.Duration,
	transport http.RoundTripper,
) Logger {
	store := NewRedisEntryRecordStore(r, logger)
	return NewLogger(
		r,
		store,
		logger,
		requestInfo,
		expiration,
		recordFullRequest,
		fullRequestExpiration,
		maxFullRequestSize,
		maxFullResponseSize,
		maxResponseWait,
		transport,
	)
}

// storeEntry persists the entry: metadata via EntryRecordStore, full JSON in Redis.
func (t *redisLogger) storeEntry(
	entry *Entry,
) error {
	er := EntryRecord{}
	t.requestInfo.setRedisRecordFields(&er)
	entry.setRedisRecordFields(&er)

	// Store the entry record metadata via the provider
	if err := t.recordStore.StoreRecord(context.Background(), &er); err != nil {
		t.logger.Error("error storing entry record", "error", err, "entry_id", entry.Id.String())
		return err
	}

	// Set expiration on the Redis hash key (for TTL-based cleanup)
	err := t.r.Expire(context.Background(), redisLogKey(entry.Id), t.expiration).Err()
	if err != nil {
		t.logger.Error("error setting expiry for log entry in Redis", "error", err, "entry_id", entry.Id.String())
		// Don't return error here - the record was stored, just TTL failed
	}

	// Store full request JSON in Redis if recording
	if t.recordFullRequest {
		jsonData, err := json.Marshal(entry)
		if err != nil {
			t.logger.Error("error serializing entry to JSON", "error", err, "entry_id", entry.Id.String())
		} else {
			err = t.r.Set(context.Background(), redisFullLogKey(entry.Id), jsonData, t.fullRequestExpiration).Err()
			if err != nil {
				t.logger.Error("error storing full HTTP log entry in Redis", "error", err, "entry_id", entry.Id.String())
			}
		}
	}

	return nil
}
