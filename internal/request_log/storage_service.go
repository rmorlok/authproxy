package request_log

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apredis"
)

type redisLogger struct {
	r                     apredis.Client
	logger                *slog.Logger
	requestInfo           RequestInfo
	expiration            time.Duration
	recordFullRequest     bool
	fullRequestExpiration time.Duration
	maxFullRequestSize    uint64        // The largest full request size to store
	maxFullResponseSize   uint64        // The largest full response size to store
	maxResponseWait       time.Duration // The longest amount of time to wait for the full response to be consumed before logging
	transport             http.RoundTripper
	persistEntry          func(*Entry) error // So test can override
}

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
	l := &redisLogger{
		r:                     r,
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
	l.persistEntry = l.storeEntryInRedis

	return l
}

// storeEntryInRedis stores the log entry in Redis. Note that this method logs errors as well as returns them because
// errors will be ignored where this is called.
func (t *redisLogger) storeEntryInRedis(
	entry *Entry,
) error {
	// Get the Redis client
	client := t.r
	pipeline := client.Pipeline()

	er := EntryRecord{}
	t.requestInfo.setRedisRecordFields(&er)
	entry.setRedisRecordFields(&er)

	vals := make(map[string]string)
	er.setRedisRecordFields(vals)

	// Store the entry
	err := pipeline.HSet(context.Background(), redisLogKey(entry.Id), vals).Err()
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.Id.String())
		return err
	}

	err = pipeline.Expire(context.Background(), redisLogKey(entry.Id), t.expiration).Err()
	if err != nil {
		t.logger.Error("error setting expiry for log entry in Redis", "error", err, "entry_id", entry.Id.String())
		return err
	}

	if t.recordFullRequest {
		jsonData, err := json.Marshal(entry)
		if err != nil {
			t.logger.Error("error serializing entry to JSON", "error", err, "entry_id", entry.Id.String())
		} else {
			err = pipeline.Set(context.Background(), redisFullLogKey(entry.Id), jsonData, t.fullRequestExpiration).Err()
			if err != nil {
				t.logger.Error("error storing full HTTP log entry in Redis", "error", err, "entry_id", entry.Id.String())
			}
		}
	}

	cmdErr, err := pipeline.Exec(context.Background())
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.Id.String())
		return err
	}

	for _, cmd := range cmdErr {
		if cmd.Err() != nil {
			t.logger.Error("error storing HTTP log entry in Redis", "error", cmd.Err(), "entry_id", entry.Id.String())
			return err
		}
	}

	return nil
}
