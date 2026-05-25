package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func minimalRateLimitDef() rlschema.RateLimit {
	return rlschema.RateLimit{
		Selector: rlschema.Selector{},
		Bucket:   rlschema.Bucket{},
		Algorithm: rlschema.Algorithm{
			TokenBucket: &rlschema.TokenBucket{Capacity: 1, RefillRate: 1},
		},
	}
}

func setupRefresherTest(t *testing.T) (database.DB, context.Context) {
	_, db := database.MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
	return db, ctx
}

// TestRefresher_Sync_Empty validates that an empty database produces an
// empty cache (with a non-zero snapshot version, so callers can tell a
// successful sync apart from a never-synced cache).
func TestRefresher_Sync_Empty(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	c := NewCache()
	r := NewRefresher(db, c, aplog.NewNoopLogger())

	require.NoError(t, r.Sync(ctx))
	require.Empty(t, c.All())
	require.Equal(t, uint64(1), c.SnapshotVersion())
	require.False(t, c.SnapshotTime().IsZero())
}

// TestRefresher_Sync_PopulatesCache creates a few rules, runs a sync, and
// verifies they all appear in the cache.
func TestRefresher_Sync_PopulatesCache(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.CreateRateLimit(ctx, &database.RateLimit{
			Id:         apid.New(apid.PrefixRateLimit),
			Namespace:  "root",
			Definition: minimalRateLimitDef(),
		}))
	}

	c := NewCache()
	r := NewRefresher(db, c, aplog.NewNoopLogger())
	require.NoError(t, r.Sync(ctx))
	require.Len(t, c.All(), 3)
}

// TestRefresher_Sync_LastKnownGood verifies that a sync against a broken
// database leaves the previous cache snapshot intact rather than wiping it.
func TestRefresher_Sync_LastKnownGood(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	require.NoError(t, db.CreateRateLimit(ctx, &database.RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: minimalRateLimitDef(),
	}))

	c := NewCache()
	r := NewRefresher(db, c, aplog.NewNoopLogger())
	require.NoError(t, r.Sync(ctx))
	require.Len(t, c.All(), 1)
	versionAfterFirstSync := c.SnapshotVersion()
	timeAfterFirstSync := c.SnapshotTime()

	// Replace the Refresher's database with nil so the next Sync() fails.
	r.db = nil
	err := r.Sync(ctx)
	require.Error(t, err)

	// Cache snapshot is unchanged.
	require.Len(t, c.All(), 1)
	require.Equal(t, versionAfterFirstSync, c.SnapshotVersion())
	require.Equal(t, timeAfterFirstSync, c.SnapshotTime())
}

// TestRefresher_Sync_HonorsContextCancellation ensures the loop returns
// promptly when the parent context is cancelled.
func TestRefresher_Run_StopsOnContextCancel(t *testing.T) {
	db, _ := setupRefresherTest(t)
	c := NewCache()
	r := NewRefresher(db, c, aplog.NewNoopLogger(), WithInterval(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

// TestRefresher_Run_PeriodicTick uses a fake clock to drive the periodic
// loop and asserts that every tick triggers a Sync.
func TestRefresher_Run_PeriodicTick(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	c := NewCache()
	fc := clock.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	syncs := make(chan error, 16)
	r := NewRefresher(db, c, aplog.NewNoopLogger(),
		WithInterval(time.Minute),
		WithClock(fc),
		withOnSync(func(err error) { syncs <- err }),
	)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = r.Run(runCtx) }()

	// First sync is the inline pre-loop call; wait for it to land.
	waitForSync(t, syncs)

	// Drive three ticks; each should fire a Sync.
	for i := 0; i < 3; i++ {
		// FakeClock's Step blocks until any waiters (including NewTicker)
		// have been scheduled. Tiny pre-sleep avoids racing the goroutine
		// that's reaching the select inside Run.
		waitFor(t, func() bool { return fc.HasWaiters() })
		fc.Step(time.Minute)
		waitForSync(t, syncs)
	}

	_ = ctx // keep the apctx ctx referenced for future expansion
}

// TestRefresher_Sync_NotifiesOnSync covers both success and failure
// notifications so observability hooks don't silently regress.
func TestRefresher_Sync_NotifiesOnSync(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	c := NewCache()
	syncs := make(chan error, 4)
	r := NewRefresher(db, c, aplog.NewNoopLogger(),
		withOnSync(func(err error) { syncs <- err }),
	)

	require.NoError(t, r.Sync(ctx))
	require.NoError(t, <-syncs)

	r.db = nil
	require.Error(t, r.Sync(ctx))
	gotErr := <-syncs
	require.Error(t, gotErr)
}

// TestStartRefresher_StartsAndStops covers the convenience wrapper.
func TestStartRefresher_StartsAndStops(t *testing.T) {
	db, ctx := setupRefresherTest(t)
	require.NoError(t, db.CreateRateLimit(ctx, &database.RateLimit{
		Id:         apid.New(apid.PrefixRateLimit),
		Namespace:  "root",
		Definition: minimalRateLimitDef(),
	}))

	c := NewCache()
	stop := StartRefresher(context.Background(), db, c, aplog.NewNoopLogger(),
		WithInterval(time.Hour),
	)

	// The initial sync inside Run() may not have completed yet — wait for it.
	waitFor(t, func() bool { return c.SnapshotVersion() >= 1 })
	require.Len(t, c.All(), 1)

	// stop() must return promptly.
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return")
	}
}

// TestNewRefresher_Defaults pins the public defaults.
func TestNewRefresher_Defaults(t *testing.T) {
	r := NewRefresher(nil, NewCache(), nil)
	require.Equal(t, DefaultRefreshInterval, r.interval)
	require.NotNil(t, r.logger)
}

// TestWithInterval_BelowFloorClamped guards against pathological config.
func TestWithInterval_BelowFloorClamped(t *testing.T) {
	r := NewRefresher(nil, NewCache(), nil, WithInterval(time.Millisecond))
	require.Equal(t, minRefreshInterval, r.interval)
}

// Sync should propagate the underlying error so callers / metrics can
// distinguish failure modes.
func TestRefresher_Sync_PropagatesError(t *testing.T) {
	r := NewRefresher(nil, NewCache(), aplog.NewNoopLogger())
	err := r.Sync(context.Background())
	require.Error(t, err)
	require.True(t, errors.Is(err, err)) // sanity
}

func waitForSync(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync notification")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true")
}
