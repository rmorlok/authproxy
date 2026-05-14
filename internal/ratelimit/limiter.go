package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
)

// Decision is the result of a per-request rate-limit check. The
// enforcement layer (#223) consumes this to decide whether to short-circuit
// with a 429 (when !Allowed) or pass the request through (when Allowed).
type Decision struct {
	// Allowed reports whether the request may proceed. When the underlying
	// Redis call fails, Allowed is true and FailedOpen is also true — see
	// the comment on FailedOpen for the rationale.
	Allowed bool

	// RetryAfter is the suggested wait time before the next request from
	// the same bucket would be permitted. Valid only when Allowed=false.
	// Window algorithms return time-remaining-in-window; token bucket
	// returns time-to-next-token.
	RetryAfter time.Duration

	// Remaining is the number of additional requests the bucket can serve
	// in the current window / from the current token pool. Valid only
	// when Allowed=true. -1 means "not computable cheaply" (e.g.
	// approximate counter mode in sliding window).
	Remaining int

	// FailedOpen indicates that the underlying counter store (Redis) was
	// unavailable and the limiter chose to allow the request rather than
	// reject it. Rate limits are guardrails, not security boundaries; a
	// Redis blip should not convert into a customer-visible outage.
	// Callers should still log / metric this so chronic Redis failures
	// remain visible.
	FailedOpen bool
}

// Limiter is the per-rule runtime evaluator. The enforcement layer holds
// one Limiter per cached RateLimit and calls Decide once per matched
// request. Implementations are stateless beyond the Redis client + the
// rule's parameters; cheap to construct, safe for concurrent use.
type Limiter interface {
	// Decide checks the counter for the given bucket and atomically either
	// increments it (Allowed=true) or returns time-until-next-allowed
	// (Allowed=false, RetryAfter set). See Decision for the failure-mode
	// contract.
	Decide(ctx context.Context, bucketKey BucketKey) (Decision, error)

	// Peek returns what Decide would return given the current counter
	// state, without writing. Used by the dry-run admin endpoint so
	// operators can validate "would this request be limited?" without
	// polluting the runtime counters. Same fail-open semantics as Decide:
	// on Redis error returns Allowed=true / FailedOpen=true.
	Peek(ctx context.Context, bucketKey BucketKey) (Decision, error)
}

// NewLimiter builds a Limiter for a RateLimit row. The algorithm variant
// is taken from rl.Definition.Algorithm; exactly one variant is required
// (schema validation enforces this at write time).
func NewLimiter(rl *database.RateLimit, redis apredis.Client, logger *slog.Logger) (Limiter, error) {
	if rl == nil {
		return nil, fmt.Errorf("ratelimit: nil rate limit")
	}
	if logger == nil {
		logger = slog.Default()
	}
	algo := rl.Definition.Algorithm

	switch {
	case algo.FixedWindow != nil:
		return newFixedWindowLimiter(rl.Id, *algo.FixedWindow, redis, logger), nil
	case algo.SlidingWindow != nil:
		return newSlidingWindowLimiter(rl.Id, *algo.SlidingWindow, redis, logger), nil
	case algo.TokenBucket != nil:
		return newTokenBucketLimiter(rl.Id, *algo.TokenBucket, redis, logger), nil
	}
	return nil, fmt.Errorf("ratelimit: rule %s has no algorithm variant set", rl.Id)
}

// limiterKeyPrefix returns the Redis key namespace shared by all
// algorithm-specific Limiters. Sharded by (rule_id, bucket_key) — the
// algorithm-specific suffix is appended by each Limiter.
func limiterKeyPrefix(ruleID apid.ID, bucketKey BucketKey) string {
	// Use a different second segment from the existing 429 round-tripper
	// store so the two systems can't collide, even if a future bug ever
	// tries to share keys.
	return fmt.Sprintf("ratelimit:rule:%s:%s", string(ruleID), bucketKey.String())
}

// failOpen is the canonical fail-open construction used by every algorithm.
// Centralised so the metric / log shape stays consistent across algorithms.
func failOpen(logger *slog.Logger, ruleID apid.ID, err error) (Decision, error) {
	logger.Warn("rate-limit counter store unavailable; failing open",
		"rule_id", ruleID,
		"error", err,
	)
	return Decision{Allowed: true, FailedOpen: true}, err
}

// Compile-time guards that the variant types implement the Limiter
// interface — kept here because Go has no formal interface declaration.
var (
	_ Limiter = (*fixedWindowLimiter)(nil)
	_ Limiter = (*slidingWindowLimiter)(nil)
	_ Limiter = (*tokenBucketLimiter)(nil)
)

