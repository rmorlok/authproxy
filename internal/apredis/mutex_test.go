package apredis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bsm/redislock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/require"
	clocktesting "k8s.io/utils/clock/testing"
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
	require.NoError(t, m2.Unlock(ctx))

	// A successfully released mutex can be reused.
	require.NoError(t, m1.Lock(ctx))
	require.NoError(t, m1.Unlock(ctx))
}

func TestMutexOptionRetryForDeadline(t *testing.T) {
	const retryFor = 200 * time.Millisecond

	t.Run("no deadline", func(t *testing.T) {
		m := &mutex{}
		MutexOptionRetryFor(retryFor)(m)

		started := time.Now()
		ctx, cancel := m.lockContextCancellation(context.Background())
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, started.Add(retryFor), deadline, 25*time.Millisecond)
	})

	t.Run("earlier deadline", func(t *testing.T) {
		m := &mutex{}
		MutexOptionRetryFor(retryFor)(m)

		parentDeadline := time.Now().Add(50 * time.Millisecond)
		parent, cancelParent := context.WithDeadline(context.Background(), parentDeadline)
		defer cancelParent()
		ctx, cancel := m.lockContextCancellation(parent)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Equal(t, parentDeadline, deadline)
	})

	t.Run("later deadline", func(t *testing.T) {
		m := &mutex{}
		MutexOptionRetryFor(retryFor)(m)

		started := time.Now()
		parent, cancelParent := context.WithDeadline(context.Background(), started.Add(time.Second))
		defer cancelParent()
		ctx, cancel := m.lockContextCancellation(parent)
		defer cancel()

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, started.Add(retryFor), deadline, 25*time.Millisecond)
	})
}

func TestRunWithMutexRenewsAndReleases(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Now())
	ctx := apctx.WithClock(context.Background(), fakeClock)
	m := newTestMutex()
	operationStarted := make(chan struct{})
	finishOperation := make(chan struct{})
	var finishOnce sync.Once
	t.Cleanup(func() { finishOnce.Do(func() { close(finishOperation) }) })
	result := make(chan error, 1)

	go func() {
		result <- RunWithMutex(ctx, m, 90*time.Second, func(context.Context) error {
			close(operationStarted)
			<-finishOperation
			return nil
		})
	}()

	<-operationStarted
	fakeClock.Step(30 * time.Second)
	require.Eventually(t, func() bool { return m.extendCallCount() == 1 }, time.Second, time.Millisecond)
	fakeClock.Step(30 * time.Second)
	require.Eventually(t, func() bool { return m.extendCallCount() == 2 }, time.Second, time.Millisecond)

	finishOnce.Do(func() { close(finishOperation) })
	require.NoError(t, <-result)
	require.Equal(t, 1, m.lockCallCount())
	require.Equal(t, 1, m.unlockCallCount())
}

func TestRunWithMutexRenewsRedisLeaseBeyondInitialDuration(t *testing.T) {
	r, err := NewMiniredis(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	const (
		key           = "renewing-mutex-integration"
		leaseDuration = 90 * time.Second
	)
	require.NoError(t, r.Del(context.Background(), key).Err())

	fakeClock := clocktesting.NewFakeClock(time.Now())
	ctx := apctx.WithClock(context.Background(), fakeClock)
	m := NewMutex(r, key, MutexOptionLockFor(leaseDuration), MutexOptionNoRetry())
	operationStarted := make(chan struct{})
	finishOperation := make(chan struct{})
	var finishOnce sync.Once
	t.Cleanup(func() { finishOnce.Do(func() { close(finishOperation) }) })
	result := make(chan error, 1)
	go func() {
		result <- RunWithMutex(ctx, m, leaseDuration, func(context.Context) error {
			close(operationStarted)
			<-finishOperation
			return nil
		})
	}()

	<-operationStarted
	for range 3 {
		miniredisServer.FastForward(40 * time.Second)
		fakeClock.Step(30 * time.Second)
		require.Eventually(t, func() bool {
			return miniredisServer.TTL(key) > 80*time.Second
		}, time.Second, time.Millisecond)
		require.Eventually(t, fakeClock.HasWaiters, time.Second, time.Millisecond,
			"renewal timer was not reset after refreshing the lease")
	}

	competitor := NewMutex(r, key, MutexOptionLockFor(leaseDuration), MutexOptionNoRetry())
	require.ErrorIs(t, competitor.Lock(context.Background()), redislock.ErrNotObtained)

	finishOnce.Do(func() { close(finishOperation) })
	require.NoError(t, <-result)
	require.False(t, miniredisServer.Exists(key))
}

func TestRunWithMutexSerializesConcurrentOperations(t *testing.T) {
	r, err := NewMiniredis(nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	const key = "serializing-mutex-integration"
	require.NoError(t, r.Del(context.Background(), key).Err())

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	secondStarted := make(chan struct{})
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)

	go func() {
		m := NewMutex(r, key, MutexOptionLockFor(time.Second), MutexOptionNoRetry())
		firstResult <- RunWithMutex(context.Background(), m, time.Second, func(context.Context) error {
			close(firstStarted)
			<-releaseFirst
			return nil
		})
	}()
	<-firstStarted

	go func() {
		m := NewMutex(
			r,
			key,
			MutexOptionLockFor(time.Second),
			MutexOptionRetryFor(time.Second),
			MutexOptionRetryLinearBackoff(time.Millisecond),
		)
		secondResult <- RunWithMutex(context.Background(), m, time.Second, func(context.Context) error {
			close(secondStarted)
			return nil
		})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second operation entered before the first released the mutex")
	case <-time.After(25 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(releaseFirst) })
	require.NoError(t, <-firstResult)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second operation did not enter after the first released the mutex")
	}
	require.NoError(t, <-secondResult)
}

func TestRunWithMutexCancelsOperationWhenLeaseIsLost(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Now())
	ctx := apctx.WithClock(context.Background(), fakeClock)
	refreshErr := errors.New("refresh rejected")
	m := newTestMutex()
	m.extendErr = refreshErr
	operationStarted := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		result <- RunWithMutex(ctx, m, 90*time.Second, func(ctx context.Context) error {
			close(operationStarted)
			<-ctx.Done()
			return context.Cause(ctx)
		})
	}()

	<-operationStarted
	fakeClock.Step(30 * time.Second)
	err := <-result
	require.ErrorIs(t, err, ErrMutexLeaseLost)
	require.ErrorIs(t, err, refreshErr)
	require.Equal(t, 1, m.unlockCallCount())
}

func TestRunWithMutexReturnsOperationAndUnlockErrors(t *testing.T) {
	operationErr := errors.New("operation failed")
	unlockErr := errors.New("unlock failed")
	m := newTestMutex()
	m.unlockErr = unlockErr

	err := RunWithMutex(context.Background(), m, time.Minute, func(context.Context) error {
		return operationErr
	})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, unlockErr)
}

func TestRunWithMutexReleasesBeforeRepanicking(t *testing.T) {
	m := newTestMutex()

	require.PanicsWithValue(t, "boom", func() {
		_ = RunWithMutex(context.Background(), m, time.Minute, func(context.Context) error {
			panic("boom")
		})
	})
	require.Equal(t, 1, m.unlockCallCount())
}

type testMutex struct {
	mu          sync.Mutex
	lockCalls   int
	extendCalls int
	unlockCalls int
	lockErr     error
	extendErr   error
	unlockErr   error
}

func newTestMutex() *testMutex {
	return &testMutex{}
}

func (m *testMutex) Lock(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lockCalls++
	return m.lockErr
}

func (m *testMutex) Extend(context.Context, time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extendCalls++
	return m.extendErr
}

func (m *testMutex) Unlock(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unlockCalls++
	return m.unlockErr
}

func (m *testMutex) lockCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockCalls
}

func (m *testMutex) extendCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.extendCalls
}

func (m *testMutex) unlockCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unlockCalls
}
