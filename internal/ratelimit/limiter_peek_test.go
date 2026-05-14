package ratelimit

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/stretchr/testify/require"
)

// The Peek tests share three invariants:
//
//  1. On a fresh state, Peek reports Allowed=true with the same Remaining
//     a real Decide call would have set.
//  2. When the bucket is saturated, Peek reports Allowed=false and a
//     plausible RetryAfter — same shape as Decide on a saturated bucket.
//  3. Calling Peek does not mutate Redis: a subsequent Decide sees the
//     bucket as if the Peek had not happened. This is the load-bearing
//     property that lets the dry-run admin endpoint hit a real
//     production-shaped counter store without polluting it.

// peekDoesNotMutate snapshots the Decide() result on a fresh state, then
// reinitialises and asserts that {Peek, Peek, Peek} followed by Decide
// returns the same Decision. Any state mutation in Peek would cause
// drift between the two Decides.
func peekDoesNotMutate(t *testing.T, env *limiterEnv, def rlschema.RateLimit) {
	t.Helper()

	baseline := mustNewLimiter(t, env, def)
	want, err := baseline.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)

	// Fresh env so the second Limiter starts from a clean Redis.
	env2 := newLimiterEnv(t)
	probe := mustNewLimiter(t, env2, def)
	for i := 0; i < 3; i++ {
		_, err := probe.Peek(env2.ctx(), mkBucket())
		require.NoError(t, err)
	}
	got, err := probe.Decide(env2.ctx(), mkBucket())
	require.NoError(t, err)

	require.Equal(t, want.Allowed, got.Allowed, "Peek must not change a subsequent Decide outcome")
	require.Equal(t, want.Remaining, got.Remaining, "Peek must not consume capacity")
}

// --- Fixed window ---

func TestPeek_FixedWindow_FreshState(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5},
	}})

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
	require.Equal(t, 4, d.Remaining, "Peek reports what Decide would set: limit - 1")
}

func TestPeek_FixedWindow_Saturated(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 2},
	}})

	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
	require.LessOrEqual(t, d.RetryAfter, time.Minute)
}

func TestPeek_FixedWindow_DoesNotMutate(t *testing.T) {
	peekDoesNotMutate(t, newLimiterEnv(t), rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5},
	}})
}

func TestPeek_FixedWindow_FailOpenOnRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	r := newFailedRedis(t)
	rl := &database.RateLimit{
		Id: apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 1},
		}},
	}
	l, err := NewLimiter(rl, r, aplog.NewNoopLogger())
	require.NoError(t, err)

	d, err := l.Peek(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

// --- Sliding window: log mode ---

func TestPeek_SlidingWindowLog_FreshState(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 4, Mode: rlschema.SlidingWindowModeLog,
		},
	}})

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
	require.Equal(t, 3, d.Remaining)
}

func TestPeek_SlidingWindowLog_Saturated(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 2, Mode: rlschema.SlidingWindowModeLog,
		},
	}})

	_, _ = l.Decide(env.ctx(), mkBucket())
	_, _ = l.Decide(env.ctx(), mkBucket())

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
}

func TestPeek_SlidingWindowLog_DoesNotMutate(t *testing.T) {
	peekDoesNotMutate(t, newLimiterEnv(t), rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 5, Mode: rlschema.SlidingWindowModeLog,
		},
	}})
}

func TestPeek_SlidingWindowLog_FailOpenOnRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	r := newFailedRedis(t)
	rl := &database.RateLimit{
		Id: apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			SlidingWindow: &rlschema.SlidingWindow{
				Window: common.HumanDuration{Duration: time.Minute}, Limit: 1, Mode: rlschema.SlidingWindowModeLog,
			},
		}},
	}
	l, _ := NewLimiter(rl, r, aplog.NewNoopLogger())

	d, err := l.Peek(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

// --- Sliding window: counter mode ---

func TestPeek_SlidingWindowCounter_FreshState(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 5, Mode: rlschema.SlidingWindowModeCounter,
		},
	}})

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
	require.Equal(t, 4, d.Remaining)
}

func TestPeek_SlidingWindowCounter_Saturated(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 2, Mode: rlschema.SlidingWindowModeCounter,
		},
	}})

	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}
	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
}

func TestPeek_SlidingWindowCounter_DoesNotMutate(t *testing.T) {
	peekDoesNotMutate(t, newLimiterEnv(t), rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 5, Mode: rlschema.SlidingWindowModeCounter,
		},
	}})
}

func TestPeek_SlidingWindowCounter_FailOpenOnRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	r := newFailedRedis(t)
	rl := &database.RateLimit{
		Id: apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			SlidingWindow: &rlschema.SlidingWindow{
				Window: common.HumanDuration{Duration: time.Minute}, Limit: 1, Mode: rlschema.SlidingWindowModeCounter,
			},
		}},
	}
	l, _ := NewLimiter(rl, r, aplog.NewNoopLogger())

	d, err := l.Peek(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

// --- Token bucket ---

func TestPeek_TokenBucket_FreshState(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1.0},
	}})

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
	require.Equal(t, 4, d.Remaining, "fresh full bucket, hypothetical consume → capacity-1")
}

func TestPeek_TokenBucket_Saturated(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 2, RefillRate: 1.0},
	}})

	// Drain.
	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}

	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
	require.LessOrEqual(t, d.RetryAfter, 2*time.Second, "1 tok/s → wait ~1s for 1 full token")
}

func TestPeek_TokenBucket_AccountsForRefill(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1.0},
	}})

	// Empty the bucket via Decide.
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.True(t, d.Allowed)
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// Peek immediately — still empty.
	d, _ = l.Peek(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// Advance enough time for a token to refill; Peek should now allow.
	env.step(1100 * time.Millisecond)
	d, err := l.Peek(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed, "after one refill interval, Peek should report Allowed without consuming")
}

func TestPeek_TokenBucket_DoesNotMutate(t *testing.T) {
	peekDoesNotMutate(t, newLimiterEnv(t), rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1.0},
	}})
}

func TestPeek_TokenBucket_FailOpenOnRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	r := newFailedRedis(t)
	rl := &database.RateLimit{
		Id: apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1.0},
		}},
	}
	l, _ := NewLimiter(rl, r, aplog.NewNoopLogger())

	d, err := l.Peek(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}
