package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
)

// slidingWindowLogScript: ZSET of (score=timestamp_ms, member=unique tag).
// Eviction: ZREMRANGEBYSCORE clears entries older than (now - window).
// Decision: ZCARD vs limit; if at/above the cap, find the oldest entry's
// score and report (oldest + window) - now as the retry window.
//
// Returns:
//
//	{1, remaining}      allowed
//	{0, retry_after_ms} rejected
var slidingWindowLogScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local now_ms = tonumber(ARGV[2])
local window_ms = tonumber(ARGV[3])
local member = ARGV[4]

redis.call('ZREMRANGEBYSCORE', key, 0, now_ms - window_ms)
local count = redis.call('ZCARD', key)

if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry_ms = window_ms
    if #oldest == 2 then
        retry_ms = (tonumber(oldest[2]) + window_ms) - now_ms
        if retry_ms < 1 then retry_ms = 1 end
    end
    return {0, retry_ms}
end

redis.call('ZADD', key, now_ms, member)
-- Slack on the TTL so an entry never gets evicted before its window
-- has fully passed; ZREMRANGEBYSCORE handles correctness.
redis.call('PEXPIRE', key, window_ms + 1000)
return {1, limit - count - 1}
`)

// slidingWindowCounterScript: weighted average of the current and
// previous fixed windows. The previous window's contribution is scaled
// by (window - elapsed_in_current) / window, modelling the request
// distribution as uniform over the previous window.
//
// Returns:
//
//	{1, remaining_approx}   allowed (remaining is approximate)
//	{0, retry_after_ms}     rejected; retry_after ~= time until prev
//	                        fully ages out
var slidingWindowCounterScript = redis.NewScript(`
local key_curr = KEYS[1]
local key_prev = KEYS[2]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local elapsed_ms = tonumber(ARGV[3])

local prev_count = tonumber(redis.call('GET', key_prev) or '0')
local curr_count = tonumber(redis.call('GET', key_curr) or '0')

-- Math here mirrors the standard sliding-window-counter approximation.
-- elapsed_ms goes 0 -> window_ms; the previous window's weight starts
-- at 1.0 (entire window still in the trailing slice) and linearly
-- decays to 0.0.
local prev_weight = (window_ms - elapsed_ms) / window_ms
if prev_weight < 0 then prev_weight = 0 end

local approx = curr_count + math.floor(prev_count * prev_weight)

if approx >= limit then
    local retry_ms = window_ms - elapsed_ms
    if retry_ms < 1 then retry_ms = 1 end
    return {0, retry_ms}
end

redis.call('INCR', key_curr)
-- 2 * window_ms keeps the previous window around long enough for the
-- next bucket's prev-lookup to find it.
redis.call('PEXPIRE', key_curr, 2 * window_ms)

local remaining = limit - approx - 1
if remaining < 0 then remaining = 0 end
return {1, remaining}
`)

type slidingWindowLimiter struct {
	ruleID  apid.ID
	limit   int
	window  time.Duration
	mode    rlschema.SlidingWindowMode
	redis   apredis.Client
	logger  *slog.Logger
}

func newSlidingWindowLimiter(ruleID apid.ID, params rlschema.SlidingWindow, r apredis.Client, logger *slog.Logger) *slidingWindowLimiter {
	return &slidingWindowLimiter{
		ruleID: ruleID,
		limit:  params.Limit,
		window: params.Window.Duration,
		mode:   params.Mode,
		redis:  r,
		logger: logger,
	}
}

func (l *slidingWindowLimiter) Decide(ctx context.Context, bucketKey BucketKey) (Decision, error) {
	now := apctx.GetClock(ctx).Now()

	switch l.mode {
	case rlschema.SlidingWindowModeLog:
		return l.decideLog(ctx, bucketKey, now)
	case rlschema.SlidingWindowModeCounter:
		return l.decideCounter(ctx, bucketKey, now)
	}
	// Schema validation should make this unreachable; treat as fail-open
	// so a bad rule doesn't bring down the proxy.
	return failOpen(l.logger, l.ruleID, fmt.Errorf("unknown sliding window mode %q", l.mode))
}

func (l *slidingWindowLimiter) decideLog(ctx context.Context, bucketKey BucketKey, now time.Time) (Decision, error) {
	key := fmt.Sprintf("%s:swl", limiterKeyPrefix(l.ruleID, bucketKey))

	// ZSET members must be unique to record multiple requests at the
	// same timestamp. Random hex is plenty — a hash of the bucket key
	// plus a counter would also work but adds complexity for no gain.
	member, err := randomHex(8)
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	windowMs := l.window.Milliseconds()
	res, err := slidingWindowLogScript.Run(ctx, l.redis,
		[]string{key},
		l.limit, now.UnixMilli(), windowMs, member,
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

func (l *slidingWindowLimiter) decideCounter(ctx context.Context, bucketKey BucketKey, now time.Time) (Decision, error) {
	windowMs := l.window.Milliseconds()
	windowID := now.UnixMilli() / windowMs
	elapsedMs := now.UnixMilli() % windowMs

	prefix := limiterKeyPrefix(l.ruleID, bucketKey)
	keyCurr := fmt.Sprintf("%s:swc:%d", prefix, windowID)
	keyPrev := fmt.Sprintf("%s:swc:%d", prefix, windowID-1)

	res, err := slidingWindowCounterScript.Run(ctx, l.redis,
		[]string{keyCurr, keyPrev},
		l.limit, windowMs, elapsedMs,
	).Result()
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}

	allowed, value, err := parseDecisionResult(res)
	if err != nil {
		return failOpen(l.logger, l.ruleID, err)
	}
	if allowed {
		// Counter-mode remaining is approximate. The Decision contract
		// allows -1 for "not computable cheaply"; we do have a number
		// here so we report it, but consumers should treat it as a
		// rough estimate not a precise budget.
		return Decision{Allowed: true, Remaining: value}, nil
	}
	return Decision{
		Allowed:    false,
		RetryAfter: time.Duration(value) * time.Millisecond,
	}, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
