package apctx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestClock(t *testing.T) {
	ctx := context.Background()

	// Real clock
	require.NotNil(t, GetClock(ctx))
	require.Less(t, GetClock(ctx).Now().Sub(time.Now()).Abs(), 1*time.Second)

	// Frozen clock
	tm := time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC)
	ctx = WithClock(ctx, clock.NewFakeClock(tm))
	require.Equal(t, tm, GetClock(ctx).Now())

	ctx = WithFixedClock(context.Background(), tm)
	require.Equal(t, tm, GetClock(ctx).Now())
}
