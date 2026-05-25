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
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// tokenBucketScript implements the standard refill-then-consume token
// bucket. State (tokens, last_refill_ms) is stored as a Redis hash so the
// two fields move atomically. New buckets are created full (tokens =
// capacity) so the first burst gets the full benefit of the bucket
// rather than starting empty.
//
// Returns:
//
//	{1, remaining_tokens_floor}  allowed
//	{0, retry_after_ms}          rejected; retry_after = ms until 1 token
//	                             would be available
//
// The refill_per_sec ARGV is encoded as a string with %g formatting so
// fractional rates (e.g. 0.5 tokens/sec) round-trip cleanly.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_per_sec = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local idle_ttl_ms = tonumber(ARGV[4])

local data = redis.call('HMGET', key, 'tokens', 'last_refill_ms')
local tokens = tonumber(data[1])
local last_refill_ms = tonumber(data[2])

if tokens == nil or last_refill_ms == nil then
    -- New bucket: start full so first-time callers get the configured
    -- burst capacity rather than instantly hitting an empty pool.
    tokens = capacity
    last_refill_ms = now_ms
else
    local elapsed_ms = now_ms - last_refill_ms
    if elapsed_ms < 0 then elapsed_ms = 0 end
    local refill = (elapsed_ms / 1000.0) * refill_per_sec
    tokens = math.min(capacity, tokens + refill)
    last_refill_ms = now_ms
end

if tokens < 1 then
    -- Not enough; calculate how many ms until 1 full token accrues at
    -- the current refill rate. Persist updated state so we don't
    -- recompute the same partial refill on the next call.
    local needed = 1 - tokens
    local wait_ms = math.ceil(needed / refill_per_sec * 1000)
    if wait_ms < 1 then wait_ms = 1 end
    redis.call('HSET', key, 'tokens', tostring(tokens), 'last_refill_ms', tostring(last_refill_ms))
    redis.call('PEXPIRE', key, idle_ttl_ms)
    return {0, wait_ms}
end

tokens = tokens - 1
redis.call('HSET', key, 'tokens', tostring(tokens), 'last_refill_ms', tostring(last_refill_ms))
redis.call('PEXPIRE', key, idle_ttl_ms)
return {1, math.floor(tokens)}
`)

// tokenBucketPeekScript mirrors tokenBucketScript but writes nothing.
// Reports what Decide would return for the current state.
//
// Returns the same {allowed, value} shape as Decide so result parsing
// can be shared.
var tokenBucketPeekScript = redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_per_sec = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])

local data = redis.call('HMGET', key, 'tokens', 'last_refill_ms')
local tokens = tonumber(data[1])
local last_refill_ms = tonumber(data[2])

if tokens == nil or last_refill_ms == nil then
    -- No state yet: Decide would create a full bucket and consume one
    -- token. Remaining = capacity - 1.
    return {1, capacity - 1}
end

local elapsed_ms = now_ms - last_refill_ms
if elapsed_ms < 0 then elapsed_ms = 0 end
local refill = (elapsed_ms / 1000.0) * refill_per_sec
local projected = math.min(capacity, tokens + refill)

if projected < 1 then
    local needed = 1 - projected
    local wait_ms = math.ceil(needed / refill_per_sec * 1000)
    if wait_ms < 1 then wait_ms = 1 end
    return {0, wait_ms}
end

-- Match Decide's post-consume Remaining: floor(projected - 1).
return {1, math.floor(projected - 1)}
`)

// tokenBucketIdleTTL is how long we keep a quiescent bucket before
// letting Redis garbage-collect it. An hour is plenty for proxy traffic
// patterns; a longer TTL just costs Redis memory.
const tokenBucketIdleTTL = time.Hour

type tokenBucketLimiter struct {
	ruleID     apid.ID
	capacity   int
	refillRate float64
	redis      apredis.Client
	logger     *slog.Logger
}

func newTokenBucketLimiter(ruleID apid.ID, params rlschema.TokenBucket, r apredis.Client, logger *slog.Logger) *tokenBucketLimiter {
	return &tokenBucketLimiter{
		ruleID:     ruleID,
		capacity:   params.Capacity,
		refillRate: params.RefillRate,
		redis:      r,
		logger:     logger,
	}
}

func (l *tokenBucketLimiter) Decide(ctx context.Context, bucketKey BucketKey) (Decision, error) {
	now := apctx.GetClock(ctx).Now()
	key := fmt.Sprintf("%s:tb", limiterKeyPrefix(l.ruleID, bucketKey))

	// Format the refill rate as a Lua-parseable float; %g preserves
	// precision for both integer and fractional values without trailing
	// zeros (e.g. "1" / "0.5" / "1.5e-06").
	rateStr := strconv.FormatFloat(l.refillRate, 'g', -1, 64)

	res, err := tokenBucketScript.Run(ctx, l.redis,
		[]string{key},
		l.capacity, rateStr, now.UnixMilli(), tokenBucketIdleTTL.Milliseconds(),
	).Result()
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	allowed, value, err := parseDecisionResult(res)
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	if allowed {
		return Decision{Allowed: true, Remaining: value}, nil
	}
	return Decision{
		Allowed:    false,
		RetryAfter: time.Duration(value) * time.Millisecond,
	}, nil
}

func (l *tokenBucketLimiter) Peek(ctx context.Context, bucketKey BucketKey) (Decision, error) {
	now := apctx.GetClock(ctx).Now()
	key := fmt.Sprintf("%s:tb", limiterKeyPrefix(l.ruleID, bucketKey))

	rateStr := strconv.FormatFloat(l.refillRate, 'g', -1, 64)

	res, err := tokenBucketPeekScript.Run(ctx, l.redis,
		[]string{key},
		l.capacity, rateStr, now.UnixMilli(),
	).Result()
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	allowed, value, err := parseDecisionResult(res)
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	if allowed {
		return Decision{Allowed: true, Remaining: value}, nil
	}
	return Decision{
		Allowed:    false,
		RetryAfter: time.Duration(value) * time.Millisecond,
	}, nil
}
