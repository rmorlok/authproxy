package request_log

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apredis"
)

const (
	fieldNamespace           = "ns"
	fieldType                = "t"
	fieldRequestId           = "id"
	fieldCorrelationId       = "cid"
	fieldTimestamp           = "ts"
	fieldDurationMs          = "dur"
	fieldConnectionId        = "cionid"
	fieldConnectorType       = "ctort"
	fieldConnectorId         = "ctorid"
	fieldConnectorVersion    = "ctorv"
	fieldMethod              = "m"
	fieldHost                = "h"
	fieldScheme              = "sch"
	fieldPath                = "p"
	fieldResponseStatusCode  = "sc"
	fieldResponseError       = "err"
	fieldRequestHttpVersion  = "reqv"
	fieldRequestSizeBytes    = "reqsz"
	fieldRequestMimeType     = "reqmt"
	fieldResponseHttpVersion = "rspv"
	fieldResponseSizeBytes   = "rspsz"
	fieldResponseMimeType    = "rspmt"
	fieldInternalTimeout     = "to"
	fieldRequestCancelled    = "rc"
	fieldFullRequestRecorded = "f"
)

func redisLogKey(requestId uuid.UUID) string {
	return fmt.Sprintf("rl:%s", requestId.String())
}

func redisFullLogKey(requestId uuid.UUID) string {
	return fmt.Sprintf("rlf:%s", requestId.String())
}

// MigrateMutexKeyName is the key that can be used when locking to perform a migration in redis.
const MigrateMutexKeyName = "request-log-migrate-lock"

const RequestLogRedisIndexName = "request_log_index_v1"

func Migrate(ctx context.Context, rs apredis.Client, l *slog.Logger) error {
	m := apredis.NewMutex(
		rs,
		MigrateMutexKeyName,
		apredis.MutexOptionLockFor(5*time.Second),
		apredis.MutexOptionRetryFor(6*time.Second),
		apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 2*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)
	err := m.Lock(context.Background())
	if err != nil {
		panic(err)
	}
	defer m.Unlock(context.Background())

	l.Info("checking if request log redis index exists")
	client := rs
	_, err = client.Info(context.Background(), RequestLogRedisIndexName).Result()
	if err == nil {
		l.Info("request log redis index already exists")
		return nil
	}

	l.Info("creating request log redis index")
	_, err = client.Do(ctx, "FT.CREATE", RequestLogRedisIndexName,
		"ON", "HASH",
		"PREFIX", "1", "rl:",
		"NOHL",
		"SCHEMA",
		fieldNamespace, "TEXT", "SORTABLE",
		fieldType, "TEXT", "NOSTEM",
		fieldCorrelationId, "TEXT", "NOSTEM",
		fieldConnectionId, "TEXT", "NOSTEM",
		fieldConnectorType, "TEXT", "NOSTEM",
		fieldConnectorId, "TEXT", "NOSTEM",
		fieldMethod, "TEXT", "NOSTEM",
		fieldPath, "TEXT", "NOSTEM",
		fieldResponseStatusCode, "NUMERIC",
		fieldConnectorVersion, "NUMERIC",
		fieldTimestamp, "NUMERIC", "SORTABLE",
	).Result()
	if err != nil {
		l.Error("failed to create request log redis index", "error", err.Error())
		return err
	}

	l.Info("request log redis index created")
	return nil
}
