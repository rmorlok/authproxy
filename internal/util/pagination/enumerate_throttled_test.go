package pagination

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// staticEnumerate produces all items in a single page, then stops.
func staticEnumerate[T any](items []T) EnumerateFunc[T] {
	return func(ctx context.Context, cb EnumerateCallback[T]) error {
		_, err := cb(PageResult[T]{Results: items})
		return err
	}
}

func TestEnumerateThrottledNilLimiter(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	var seen []int
	err := EnumerateThrottled(context.Background(), staticEnumerate(items), nil, func(v int) error {
		seen = append(seen, v)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, items, seen)
}

func TestEnumerateThrottledStopsOnCallbackError(t *testing.T) {
	items := []int{1, 2, 3}
	stopErr := errors.New("stop")
	var seen []int
	err := EnumerateThrottled(context.Background(), staticEnumerate(items), nil, func(v int) error {
		seen = append(seen, v)
		if v == 2 {
			return stopErr
		}
		return nil
	})
	require.ErrorIs(t, err, stopErr)
	require.Equal(t, []int{1, 2}, seen)
}

func TestEnumerateThrottledRespectsLimiter(t *testing.T) {
	// 50 records/sec, burst of 1 — three items should take roughly two
	// limiter intervals (~40ms) before the third callback runs.
	limiter := rate.NewLimiter(50, 1)
	items := []int{1, 2, 3}

	start := time.Now()
	err := EnumerateThrottled(context.Background(), staticEnumerate(items), limiter, func(int) error {
		return nil
	})
	elapsed := time.Since(start)
	require.NoError(t, err)
	// Two waits at 50 rps = ~40ms minimum. Allow generous slack for CI noise.
	require.GreaterOrEqual(t, elapsed, 30*time.Millisecond, "limiter should slow throughput")
}

func TestEnumerateThrottledStopsOnLimiterError(t *testing.T) {
	// Burst of 1, very long interval. Drain the burst then run with a
	// context already past its deadline so Wait fails immediately.
	limiter := rate.NewLimiter(rate.Every(time.Hour), 1)
	require.NoError(t, limiter.Wait(context.Background()))

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	called := 0
	err := EnumerateThrottled(ctx, staticEnumerate([]int{1, 2}), limiter, func(int) error {
		called++
		return nil
	})
	require.Error(t, err)
	require.Equal(t, 0, called, "callback should not fire when the limiter rejects")
}
