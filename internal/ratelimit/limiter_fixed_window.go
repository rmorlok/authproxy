package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
)

// fixedWindowScript atomically increments the counter for the current
// window. The first request to land in a window establishes the TTL —
// subsequent requests inherit it. Returns:
//
//	{1, remaining}            when allowed (count <= limit)
//	{0, retry_after_ms}       when exceeded; retry_after = remaining ms
//	                          in the current window (PTTL)
//
// The window key incorporates floor(now_ms/window_ms) so windows roll
// without any explicit deletion — old keys expire on their own TTL.
var fixedWindowScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local count = redis.call('INCR', key)
if count == 1 then
    redis.call('PEXPIRE', key, window_ms)
end

if count > limit then
    local pttl = redis.call('PTTL', key)
    if pttl < 0 then
        -- TTL not set yet (race against an EXPIRE that hasn't landed):
        -- fall back to the full window so callers don't see -1.
        pttl = window_ms
    end
    return {0, pttl}
end

return {1, limit - count}
`)

type fixedWindowLimiter struct {
	ruleID   apid.ID
	limit    int
	window   time.Duration
	redis    apredis.Client
	logger   *slog.Logger
}

func newFixedWindowLimiter(ruleID apid.ID, params rlschema.FixedWindow, r apredis.Client, logger *slog.Logger) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		ruleID: ruleID,
		limit:  params.Limit,
		window: params.Window.Duration,
		redis:  r,
		logger: logger,
	}
}

func (l *fixedWindowLimiter) Decide(ctx context.Context, bucketKey BucketKey) (Decision, error) {
	now := apctx.GetClock(ctx).Now()
	windowMs := l.window.Milliseconds()

	// Bucket the current request to the floor(now / window) window. Old
	// windows' keys self-expire, so we never need to clean them up.
	windowID := now.UnixMilli() / windowMs
	key := fmt.Sprintf("%s:fw:%d", limiterKeyPrefix(l.ruleID, bucketKey), windowID)

	res, err := fixedWindowScript.Run(ctx, l.redis, []string{key}, l.limit, windowMs).Result()
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	allowed, retryOrRemainingMs, err := parseDecisionResult(res)
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	if allowed {
		return Decision{Allowed: true, Remaining: retryOrRemainingMs}, nil
	}
	return Decision{
		Allowed:    false,
		RetryAfter: time.Duration(retryOrRemainingMs) * time.Millisecond,
	}, nil
}

// parseDecisionResult unpacks the {allowed, value} pair returned by
// every Lua script in this package. Centralised so the unmarshaling
// edge cases (Redis returns int64 vs string depending on transport)
// only need handling once.
func parseDecisionResult(res interface{}) (bool, int, error) {
	arr, ok := res.([]interface{})
	if !ok || len(arr) != 2 {
		return false, 0, fmt.Errorf("unexpected lua result shape: %#v", res)
	}
	allowed, err := toInt(arr[0])
	if err != nil {
		return false, 0, fmt.Errorf("allowed: %w", err)
	}
	value, err := toInt(arr[1])
	if err != nil {
		return false, 0, fmt.Errorf("value: %w", err)
	}
	return allowed == 1, value, nil
}

// toInt coerces the Lua-side number into a Go int. The redis driver
// returns Lua integers as int64 in modern versions, but older code paths
// (and miniredis) sometimes deliver strings — handle both.
func toInt(v interface{}) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}
	return 0, fmt.Errorf("not an int: %T", v)
}
