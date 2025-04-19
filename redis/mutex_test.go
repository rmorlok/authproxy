package redis

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMutex(t *testing.T) {
	r, err := NewMiniredis(nil, &config.KeyDataRandomBytes{}, test_utils.NewTestLogger())
	require.NoError(t, err)
	ctx := context.Background()

	m1 := r.NewMutex(
		"some-mutex",
		MutexOptionLockFor(250*time.Millisecond),
		MutexOptionRetryFor(100*time.Millisecond),
		MutexOptionRetryExponentialBackoff(30*time.Millisecond, 400*time.Millisecond),
		MutexOptionDetailedLockMetadata(),
	)

	m2 := r.NewMutex(
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
