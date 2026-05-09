package ratelimit

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/require"
)

func mkRule(id string) *database.RateLimit {
	return &database.RateLimit{Id: apid.ID(id)}
}

func TestCache_EmptyByDefault(t *testing.T) {
	c := NewCache()
	require.Empty(t, c.All())
	require.True(t, c.SnapshotTime().IsZero())
	require.Equal(t, uint64(0), c.SnapshotVersion())
}

func TestCache_Replace(t *testing.T) {
	c := NewCache()
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	c.Replace([]*database.RateLimit{mkRule("rl_a"), mkRule("rl_b")}, t1)
	require.Equal(t, t1, c.SnapshotTime())
	require.Equal(t, uint64(1), c.SnapshotVersion())
	require.Len(t, c.All(), 2)

	c.Replace([]*database.RateLimit{mkRule("rl_c")}, t2)
	require.Equal(t, t2, c.SnapshotTime())
	require.Equal(t, uint64(2), c.SnapshotVersion())
	all := c.All()
	require.Len(t, all, 1)
	require.Equal(t, apid.ID("rl_c"), all[0].Id)
}

// TestCache_AllReturnsCopy verifies that mutating the slice returned by All()
// does not affect the cache, even though we use a shared underlying slice
// internally.
func TestCache_AllReturnsCopy(t *testing.T) {
	c := NewCache()
	c.Replace([]*database.RateLimit{mkRule("rl_a")}, time.Now())

	all := c.All()
	all[0] = mkRule("rl_TAMPERED")
	all = append(all, mkRule("rl_extra"))
	_ = all

	got := c.All()
	require.Len(t, got, 1)
	require.Equal(t, apid.ID("rl_a"), got[0].Id, "callers should not be able to mutate the cache via All()")
}

// TestCache_ConcurrentReadsAndReplace runs many concurrent readers against an
// active replacer to confirm read-side safety. Race detector (-race) catches
// any data race.
func TestCache_ConcurrentReadsAndReplace(t *testing.T) {
	c := NewCache()
	c.Replace([]*database.RateLimit{mkRule("rl_initial")}, time.Now())

	const readers = 16
	const iterations = 500

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var swaps atomic.Uint64

	// Replacer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				rules := []*database.RateLimit{
					mkRule("rl_a"), mkRule("rl_b"), mkRule("rl_c"),
				}
				c.Replace(rules, time.Now())
				swaps.Add(1)
			}
		}
	}()

	// Readers
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				all := c.All()
				_ = all
				_ = c.SnapshotTime()
				_ = c.SnapshotVersion()
			}
		}()
	}

	// Let things spin briefly.
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	require.Greater(t, swaps.Load(), uint64(0))
}

// TestCache_VersionMonotonic ensures Replace always advances the version
// counter, even if the supplied slice is identical to the previous one.
func TestCache_VersionMonotonic(t *testing.T) {
	c := NewCache()
	rules := []*database.RateLimit{mkRule("rl_a")}
	for i := 1; i <= 10; i++ {
		c.Replace(rules, time.Now())
		require.Equal(t, uint64(i), c.SnapshotVersion())
	}
}
