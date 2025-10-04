package apredis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMutex(t *testing.T) {
	r, err := NewMiniredis(nil)
	require.NoError(t, err)
	ctx := context.Background()

	m1 := NewMutex(
		r,
		"some-mutex",
		MutexOptionLockFor(250*time.Millisecond),
		MutexOptionRetryFor(100*time.Millisecond),
		MutexOptionRetryExponentialBackoff(30*time.Millisecond, 400*time.Millisecond),
		MutexOptionDetailedLockMetadata(),
	)

	m2 := NewMutex(
		r,
		"some-mutex",
		MutexOptionLockFor(250*time.Millisecond),
		MutexOptionNoRetry(),
	)

	err = m1.Lock(ctx)
	require.NoError(t, err)

	err = m2.Lock(ctx)
	require.True(t, MutexIsErrNotObtained(err))

	err = m1.Unlock(ctx)
	require.NoError(t, err)

	err = m2.Lock(ctx)
	require.NoError(t, err)
}
