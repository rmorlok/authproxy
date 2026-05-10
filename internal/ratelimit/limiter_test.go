package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	rlschema "github.com/rmorlok/authproxy/internal/schema/rate_limit"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// limiterEnv bundles the moving parts every limiter test needs:
// a real apredis.Client backed by miniredis + a fake clock at a stable
// epoch. Tests step the clock and FastForward miniredis in sync via
// step() so TTL-driven behaviour matches what would happen against
// real Redis.
type limiterEnv struct {
	rds    apredis.Client
	server *miniredis.Miniredis
	clock  *clock.FakeClock
}

func newLimiterEnv(t *testing.T) *limiterEnv {
	t.Helper()
	_, r, server := apredis.MustApplyTestConfigWithServer(nil)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fc := clock.NewFakeClock(t0)
	return &limiterEnv{rds: r, server: server, clock: fc}
}

func (e *limiterEnv) ctx() context.Context {
	return apctx.NewBuilderBackground().WithClock(e.clock).Build()
}

// step advances both the fake Go clock and the miniredis server clock.
// Keeping them in sync lets TTL-driven tests behave the same way they
// would against real Redis.
func (e *limiterEnv) step(d time.Duration) {
	e.clock.Step(d)
	e.server.FastForward(d)
}

func mkBucket() BucketKey {
	return BucketKey{Components: []BucketKeyComponent{{Name: "actor", Value: "act_test"}}}
}

func mustNewLimiter(t *testing.T, env *limiterEnv, def rlschema.RateLimit) Limiter {
	t.Helper()
	rl := &database.RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Definition: def,
	}
	l, err := NewLimiter(rl, env.rds, aplog.NewNoopLogger())
	require.NoError(t, err)
	return l
}

// TestNewLimiter_DispatchesByVariant pins the algorithm-selection logic.
func TestNewLimiter_DispatchesByVariant(t *testing.T) {
	env := newLimiterEnv(t)
	cases := []struct {
		name string
		def  rlschema.RateLimit
	}{
		{"fixed_window", rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5},
		}}},
		{"sliding_window_log", rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			SlidingWindow: &rlschema.SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5, Mode: rlschema.SlidingWindowModeLog},
		}}},
		{"sliding_window_counter", rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			SlidingWindow: &rlschema.SlidingWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 5, Mode: rlschema.SlidingWindowModeCounter},
		}}},
		{"token_bucket", rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1.0},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := mustNewLimiter(t, env, tc.def)
			require.NotNil(t, l)
		})
	}
}

func TestNewLimiter_NoVariantSet(t *testing.T) {
	env := newLimiterEnv(t)
	rl := &database.RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{},
	}
	_, err := NewLimiter(rl, env.rds, aplog.NewNoopLogger())
	require.Error(t, err)
}

func TestNewLimiter_NilRateLimit(t *testing.T) {
	env := newLimiterEnv(t)
	_, err := NewLimiter(nil, env.rds, aplog.NewNoopLogger())
	require.Error(t, err)
}

// --- Fixed window ---

func TestFixedWindow_BelowLimitAllowed(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 3},
	}})
	for i := 0; i < 3; i++ {
		d, err := l.Decide(env.ctx(), mkBucket())
		require.NoError(t, err)
		require.True(t, d.Allowed, "request %d should be allowed", i+1)
		require.Equal(t, 3-(i+1), d.Remaining)
	}
}

func TestFixedWindow_ExceedsLimitRejected(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 2},
	}})
	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
	require.LessOrEqual(t, d.RetryAfter, time.Minute)
}

func TestFixedWindow_WindowRollover(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 2},
	}})
	// Saturate the first window.
	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// Cross the window boundary; the next request should be allowed.
	env.step(time.Minute + time.Second)
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed, "should be allowed in the new window")
}

func TestFixedWindow_PerBucketIsolation(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Minute}, Limit: 1},
	}})
	bucketA := BucketKey{Components: []BucketKeyComponent{{Name: "actor", Value: "a"}}}
	bucketB := BucketKey{Components: []BucketKeyComponent{{Name: "actor", Value: "b"}}}

	// Bucket A consumes its quota.
	d, _ := l.Decide(env.ctx(), bucketA)
	require.True(t, d.Allowed)
	d, _ = l.Decide(env.ctx(), bucketA)
	require.False(t, d.Allowed)

	// Bucket B is unaffected.
	d, _ = l.Decide(env.ctx(), bucketB)
	require.True(t, d.Allowed)
}

// --- Sliding window: log mode ---

func TestSlidingWindowLog_BasicAllowReject(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 2, Mode: rlschema.SlidingWindowModeLog,
		},
	}})

	d, _ := l.Decide(env.ctx(), mkBucket())
	require.True(t, d.Allowed)
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.True(t, d.Allowed)

	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
}

func TestSlidingWindowLog_OldEntriesEvicted(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 2, Mode: rlschema.SlidingWindowModeLog,
		},
	}})

	// Two requests at t=0; saturate.
	_, _ = l.Decide(env.ctx(), mkBucket())
	_, _ = l.Decide(env.ctx(), mkBucket())
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// Step past the window; the trailing window now contains zero
	// entries → next request is allowed.
	env.step(time.Minute + time.Second)
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
}

func TestSlidingWindowLog_RetryAfterShrinksAsTimePasses(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 1, Mode: rlschema.SlidingWindowModeLog,
		},
	}})

	d, _ := l.Decide(env.ctx(), mkBucket())
	require.True(t, d.Allowed)

	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
	first := d.RetryAfter

	env.step(30 * time.Second)
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
	require.Less(t, d.RetryAfter, first, "retry-after should shrink as time passes")
}

// --- Sliding window: counter mode ---

func TestSlidingWindowCounter_BasicAllowReject(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: time.Minute}, Limit: 3, Mode: rlschema.SlidingWindowModeCounter,
		},
	}})

	for i := 0; i < 3; i++ {
		d, err := l.Decide(env.ctx(), mkBucket())
		require.NoError(t, err)
		require.True(t, d.Allowed, "request %d should be allowed", i+1)
	}

	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.False(t, d.Allowed)
}

func TestSlidingWindowCounter_PreviousWindowDecays(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		SlidingWindow: &rlschema.SlidingWindow{
			Window: common.HumanDuration{Duration: 60 * time.Second}, Limit: 4, Mode: rlschema.SlidingWindowModeCounter,
		},
	}})

	// Saturate window N at t=0.
	for i := 0; i < 4; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}

	// Step into the next window (t=60s). Approx count =
	// new_count(0) + prev_count(4) * (window-elapsed=60)/60 = 4 → still rejected.
	env.step(time.Minute)
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed, "boundary of next window: full prev-window weight, still saturated")

	// Step half a window further (t=90s). Approx ≈ 0 + 4 * 30/60 = 2 → allowed.
	env.step(30 * time.Second)
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed, "halfway through next window: prev contributes 2, allowed")
}

// --- Token bucket ---

func TestTokenBucket_BurstThenLimit(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 3, RefillRate: 1.0},
	}})

	// Initial burst — capacity tokens available.
	for i := 0; i < 3; i++ {
		d, err := l.Decide(env.ctx(), mkBucket())
		require.NoError(t, err)
		require.True(t, d.Allowed, "burst slot %d", i+1)
	}

	// 4th call drains the bucket.
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
	require.GreaterOrEqual(t, d.RetryAfter, 900*time.Millisecond)
	require.LessOrEqual(t, d.RetryAfter, 1100*time.Millisecond)
}

func TestTokenBucket_Refill(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 2, RefillRate: 2.0},
	}})

	for i := 0; i < 2; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// 500ms at 2 tokens/sec = 1 full token.
	env.step(500 * time.Millisecond)
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)

	// Bucket should be empty again immediately.
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
}

func TestTokenBucket_RefillCappedAtCapacity(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 5, RefillRate: 1.0},
	}})

	// Drain.
	for i := 0; i < 5; i++ {
		_, _ = l.Decide(env.ctx(), mkBucket())
	}
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)

	// Wait far longer than capacity/rate; bucket should re-fill to
	// capacity, not beyond.
	env.step(time.Hour)
	for i := 0; i < 5; i++ {
		d, err := l.Decide(env.ctx(), mkBucket())
		require.NoError(t, err)
		require.True(t, d.Allowed, "refill slot %d", i+1)
	}
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed, "burst should be capped at Capacity, not unbounded")
}

func TestTokenBucket_FractionalRefillRate(t *testing.T) {
	env := newLimiterEnv(t)
	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 0.5},
	}})

	// Single token consumed.
	d, _ := l.Decide(env.ctx(), mkBucket())
	require.True(t, d.Allowed)
	d, _ = l.Decide(env.ctx(), mkBucket())
	require.False(t, d.Allowed)
	// Refill rate 0.5/s → 1 token in 2s.
	require.GreaterOrEqual(t, d.RetryAfter, 1900*time.Millisecond)
	require.LessOrEqual(t, d.RetryAfter, 2100*time.Millisecond)

	env.step(2 * time.Second)
	d, err := l.Decide(env.ctx(), mkBucket())
	require.NoError(t, err)
	require.True(t, d.Allowed)
}

// --- Fail-open ---

// failingRedis is a minimal stub that satisfies apredis.Client by
// embedding a real client and overriding the script entry point to
// always error. We can't spin up a "broken" miniredis cleanly, so we
// close the underlying server and let real call attempts fail.
func newFailedRedis(t *testing.T) apredis.Client {
	_, r, server := apredis.MustApplyTestConfigWithServer(nil)
	server.Close() // subsequent commands should error
	return r
}

func TestFixedWindow_FailOpenOnRedisDown(t *testing.T) {
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

	d, err := l.Decide(env.ctx(), mkBucket())
	require.Error(t, err, "Redis-down decide should propagate the error")
	require.True(t, d.Allowed, "fail-open: allow when Redis is unavailable")
	require.True(t, d.FailedOpen)
}

func TestSlidingWindowLog_FailOpenOnRedisDown(t *testing.T) {
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

	d, err := l.Decide(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

func TestSlidingWindowCounter_FailOpenOnRedisDown(t *testing.T) {
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

	d, err := l.Decide(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

func TestTokenBucket_FailOpenOnRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	r := newFailedRedis(t)

	rl := &database.RateLimit{
		Id: apid.New(apid.PrefixRateLimit),
		Definition: rlschema.RateLimit{Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1.0},
		}},
	}
	l, _ := NewLimiter(rl, r, aplog.NewNoopLogger())

	d, err := l.Decide(env.ctx(), mkBucket())
	require.Error(t, err)
	require.True(t, d.Allowed)
	require.True(t, d.FailedOpen)
}

// TestConcurrentDecidesUnderRace runs many goroutines hammering a single
// rule + bucket to confirm the Lua scripts produce a consistent count
// (no races, no double-allow). With limit=N over many goroutines, the
// number of allowed decisions should be exactly N.
func TestFixedWindow_ConcurrentDecides(t *testing.T) {
	env := newLimiterEnv(t)
	const limit = 50
	const goroutines = 200

	l := mustNewLimiter(t, env, rlschema.RateLimit{Algorithm: rlschema.Algorithm{
		FixedWindow: &rlschema.FixedWindow{Window: common.HumanDuration{Duration: time.Hour}, Limit: limit},
	}})

	var allowed int
	var rejected int
	allowedCh := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			d, err := l.Decide(env.ctx(), mkBucket())
			if err != nil {
				allowedCh <- false
				return
			}
			allowedCh <- d.Allowed
		}()
	}
	for i := 0; i < goroutines; i++ {
		if <-allowedCh {
			allowed++
		} else {
			rejected++
		}
	}
	require.Equal(t, limit, allowed, "exactly Limit decisions should be Allowed under concurrent load")
	require.Equal(t, goroutines-limit, rejected)
}
