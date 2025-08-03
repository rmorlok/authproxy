package request_log

import (
	"context"
	"github.com/rmorlok/authproxy/redis"
	"log/slog"
	"net/http"
	"time"
)

type redisLogger struct {
	r                     redis.R
	logger                *slog.Logger
	requestInfo           RequestInfo
	expiration            time.Duration
	recordFullRequest     bool
	fullRequestExpiration time.Duration
	transport             http.RoundTripper
}

func NewRedisLogger(
	r redis.R,
	logger *slog.Logger,
	requestInfo RequestInfo,
	expiration time.Duration,
	recordFullRequest bool,
	fullRequestExpiration time.Duration,
	transport http.RoundTripper,
) Logger {
	return &redisLogger{
		r:                     r,
		logger:                logger,
		requestInfo:           requestInfo,
		expiration:            expiration,
		recordFullRequest:     recordFullRequest,
		fullRequestExpiration: fullRequestExpiration,
		transport:             transport,
	}
}

// storeEntryInRedis stores the log entry in Redis
func (t *redisLogger) storeEntryInRedis(entry *Entry) {
	// Get the Redis client
	client := t.r.Client()
	pipeline := client.Pipeline()

	vals := make(map[string]interface{})
	t.requestInfo.setRedisRecordFields(vals)
	entry.setRedisRecordFields(vals)

	// Convert the entry to JSON
	//entryJSON, err := json.Marshal(entry)
	//if err != nil {
	//	t.logger.Error("error marshaling HTTP log entry", "error", err, "entry_id", entry.ID.String())
	//	return
	//}

	// Store the entry
	err := pipeline.HSet(context.Background(), redisLogKey(entry.ID), vals).Err()
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return
	}

	err = pipeline.Expire(context.Background(), redisLogKey(entry.ID), t.expiration).Err()
	if err != nil {
		t.logger.Error("error setting expiry for log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return
	}

	cmdErr, err := pipeline.Exec(context.Background())
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return
	}

	for _, cmd := range cmdErr {
		if cmd.Err() != nil {
			t.logger.Error("error storing HTTP log entry in Redis", "error", cmd.Err(), "entry_id", entry.ID.String())
			return
		}
	}
}
