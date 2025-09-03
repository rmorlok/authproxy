package request_log

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/redis"
)

type redisLogger struct {
	r                     redis.R
	logger                *slog.Logger
	requestInfo           RequestInfo
	expiration            time.Duration
	recordFullRequest     bool
	fullRequestExpiration time.Duration
	maxFullRequestSize    uint64
	maxFullResponseSize   uint64
	transport             http.RoundTripper
}

func NewRedisLogger(
	r redis.R,
	logger *slog.Logger,
	requestInfo RequestInfo,
	expiration time.Duration,
	recordFullRequest bool,
	fullRequestExpiration time.Duration,
	maxFullRequestSize uint64,
	maxFullResponseSize uint64,
	transport http.RoundTripper,
) Logger {
	return &redisLogger{
		r:                     r,
		logger:                logger,
		requestInfo:           requestInfo,
		expiration:            expiration,
		recordFullRequest:     recordFullRequest,
		fullRequestExpiration: fullRequestExpiration,
		maxFullRequestSize:    maxFullRequestSize,
		maxFullResponseSize:   maxFullResponseSize,
		transport:             transport,
	}
}

// storeEntryInRedis stores the log entry in Redis. Note that this method logs errors as well as returns them because
// errors will be ignored where this is called.
func (t *redisLogger) storeEntryInRedis(
	entry *Entry,
	requestBodyBuf *bytes.Buffer,
	responseBodyReader *io.PipeReader,
) error {
	// Get the Redis client
	client := t.r.Client()
	pipeline := client.Pipeline()

	er := EntryRecord{}
	t.requestInfo.setRedisRecordFields(&er)
	entry.setRedisRecordFields(&er)

	vals := make(map[string]interface{})
	er.setRedisRecordFields(vals)

	// Store the entry
	err := pipeline.HSet(context.Background(), redisLogKey(entry.ID), vals).Err()
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return err
	}

	err = pipeline.Expire(context.Background(), redisLogKey(entry.ID), t.expiration).Err()
	if err != nil {
		t.logger.Error("error setting expiry for log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return err
	}

	if t.recordFullRequest {
		if requestBodyBuf != nil {
			requestData, err := io.ReadAll(requestBodyBuf)
			if err != nil {
				t.logger.Error("error reading full request body", "error", err, "entry_id", entry.ID.String())
				entry.Request.Body = []byte(err.Error())
			} else {
				entry.Request.Body = requestData
			}
		}

		if responseBodyReader != nil {
			responseData, err := io.ReadAll(responseBodyReader)
			if err != nil {
				t.logger.Error("error reading full request body", "error", err, "entry_id", entry.ID.String())
				entry.Response.Body = []byte(err.Error())
			} else {
				entry.Response.Body = responseData
			}
		}

		jsonData, err := json.Marshal(entry)
		if err != nil {
			t.logger.Error("error serializing entry to JSON", "error", err, "entry_id", entry.ID.String())
		} else {
			err = pipeline.Set(context.Background(), redisFullLogKey(entry.ID), jsonData, t.fullRequestExpiration).Err()
			if err != nil {
				t.logger.Error("error storing full HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String())
			}
		}
	}

	cmdErr, err := pipeline.Exec(context.Background())
	if err != nil {
		t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String())
		return err
	}

	for _, cmd := range cmdErr {
		if cmd.Err() != nil {
			t.logger.Error("error storing HTTP log entry in Redis", "error", cmd.Err(), "entry_id", entry.ID.String())
			return err
		}
	}

	return nil
}
