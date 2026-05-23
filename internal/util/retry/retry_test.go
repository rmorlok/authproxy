package retry

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tclock "k8s.io/utils/clock/testing"
)

func TestDo_SuccessFirstTry(t *testing.T) {
	ctx := context.Background()

	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 0},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", res.Value)
	assert.Equal(t, 1, res.Attempts)
	assert.Equal(t, 1, calls)
}

func TestDo_RetriesUntilSuccess(t *testing.T) {
	ctx := context.Background()

	calls := 0
	res, err := Do(ctx, Options[int]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
	}, func(ctx context.Context) (int, error) {
		calls++
		if calls < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})

	require.NoError(t, err)
	assert.Equal(t, 42, res.Value)
	assert.Equal(t, 3, res.Attempts)
}

func TestDo_ExhaustsBudgetReturnsLastError(t *testing.T) {
	ctx := context.Background()

	wantErr := errors.New("always fails")
	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "last", wantErr
	})

	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, "last", res.Value, "Result.Value carries last attempt's value on exhaustion")
	assert.Equal(t, 3, res.Attempts)
	assert.Equal(t, 3, calls)
}

func TestDo_ClassifierFalseStopsImmediately(t *testing.T) {
	ctx := context.Background()

	wantErr := errors.New("permanent")
	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 5,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
		Classify: func(_ string, err error) bool {
			return false
		},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "v", wantErr
	})

	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, calls, "non-retryable error must not be retried")
	assert.Equal(t, 1, res.Attempts)
}

func TestDo_ClassifierInspectsValue(t *testing.T) {
	// Mimics the OAuth callsites: retry on err OR resp.StatusCode >= 500.
	ctx := context.Background()

	type fakeResp struct{ status int }

	calls := 0
	res, err := Do(ctx, Options[*fakeResp]{
		MaxAttempts: 4,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
		Classify: func(v *fakeResp, err error) bool {
			return err != nil || (v != nil && v.status >= 500)
		},
	}, func(ctx context.Context) (*fakeResp, error) {
		calls++
		if calls < 3 {
			return &fakeResp{status: 503}, nil
		}
		return &fakeResp{status: 200}, nil
	})

	require.NoError(t, err)
	require.NotNil(t, res.Value)
	assert.Equal(t, 200, res.Value.status)
	assert.Equal(t, 3, res.Attempts)
}

func TestDo_OnRetryCalledOnceLessThanAttemptsOnExhaustion(t *testing.T) {
	ctx := context.Background()

	var hookCalls int32
	_, _ = Do(ctx, Options[string]{
		MaxAttempts: 4,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
		OnRetry: func(attempt int, _ string, err error) {
			atomic.AddInt32(&hookCalls, 1)
		},
	}, func(ctx context.Context) (string, error) {
		return "", errors.New("transient")
	})

	assert.Equal(t, int32(3), atomic.LoadInt32(&hookCalls),
		"OnRetry fires before each retry — MaxAttempts-1 times on exhaustion")
}

func TestDo_OnRetryReceivesAttemptNumber(t *testing.T) {
	ctx := context.Background()

	var seen []int
	_, _ = Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
		OnRetry: func(attempt int, _ string, _ error) {
			seen = append(seen, attempt)
		},
	}, func(ctx context.Context) (string, error) {
		return "", errors.New("transient")
	})

	assert.Equal(t, []int{1, 2}, seen, "OnRetry receives the attempt number that just failed")
}

func TestDo_CtxCancelBeforeFirstCallReturnsCtxErr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 0},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "", nil
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, calls)
	assert.Equal(t, 0, res.Attempts)
}

func TestDo_CtxCancelDuringBackoffReturnsCtxErrWithCompletedAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 5,
		Backoff:     &backoff.ConstantBackOff{Interval: 50 * time.Millisecond},
	}, func(ctx context.Context) (string, error) {
		calls++
		if calls == 2 {
			go cancel()
		}
		return "v", errors.New("transient")
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 2, calls, "ctx cancel during the 2nd backoff should abort before the 3rd call")
	assert.Equal(t, 2, res.Attempts, "Attempts reflects completed op invocations only")
}

func TestDo_UsesInjectedClock(t *testing.T) {
	// Verifies the helper sleeps through apctx.GetClock(ctx) rather than
	// time.After directly — this is the testability hook that lets
	// downstream sites (notably the disconnect revoke loop) drop in.
	fakeClk := tclock.NewFakeClock(time.Now())
	ctx := apctx.WithClock(context.Background(), fakeClk)

	const backoff_ = 10 * time.Second // huge — would hang on a real clock
	calls := make(chan int, 5)

	done := make(chan struct {
		res Result[string]
		err error
	}, 1)
	go func() {
		res, err := Do(ctx, Options[string]{
			MaxAttempts: 3,
			Backoff:     &backoff.ConstantBackOff{Interval: backoff_},
		}, func(ctx context.Context) (string, error) {
			calls <- 1
			return "", errors.New("transient")
		})
		done <- struct {
			res Result[string]
			err error
		}{res, err}
	}()

	// First call.
	<-calls
	// Helper is now waiting on clk.After(backoff_).
	require.Eventually(t, fakeClk.HasWaiters, time.Second, time.Millisecond)
	fakeClk.Step(backoff_)

	// Second call.
	<-calls
	require.Eventually(t, fakeClk.HasWaiters, time.Second, time.Millisecond)
	fakeClk.Step(backoff_)

	// Third (final) call.
	<-calls

	r := <-done
	require.Error(t, r.err)
	assert.Equal(t, 3, r.res.Attempts)
}

func TestDo_BackoffStopTerminates(t *testing.T) {
	// If the backoff strategy returns Stop, the loop ends with the last
	// error — even if the budget isn't exhausted.
	ctx := context.Background()

	wantErr := errors.New("transient")
	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 10,
		Backoff:     &backoff.StopBackOff{},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "", wantErr
	})

	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, calls, "Stop from backoff terminates after the failing attempt")
	assert.Equal(t, 1, res.Attempts, "Attempts reflects completed op invocations, not the unused budget")
}

func TestDo_DefaultClassifierRetriesOnError(t *testing.T) {
	ctx := context.Background()

	calls := 0
	_, err := Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "", errors.New("nope")
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls)
}

func TestDo_MaxAttemptsBelowOneCoercedToOne(t *testing.T) {
	ctx := context.Background()

	calls := 0
	res, err := Do(ctx, Options[string]{
		MaxAttempts: 0,
		Backoff:     &backoff.ConstantBackOff{Interval: 0},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 1, res.Attempts)
}

func TestDo_OnRetryWaitOverridesBackoff(t *testing.T) {
	// Verifies that an OnRetryWait override takes precedence over the
	// Backoff strategy's wait, while leaving the Backoff progression in
	// place (it's still advanced via NextBackOff each iteration).
	fakeClk := tclock.NewFakeClock(time.Now())
	ctx := apctx.WithClock(context.Background(), fakeClk)

	const huge = time.Hour      // Backoff returns this — would hang the test
	const override = 7 * time.Second

	calls := make(chan int, 5)
	done := make(chan struct{}, 1)

	go func() {
		_, _ = Do(ctx, Options[string]{
			MaxAttempts: 3,
			Backoff:     &backoff.ConstantBackOff{Interval: huge},
			OnRetryWait: func(_ string, _ error) time.Duration {
				return override
			},
		}, func(ctx context.Context) (string, error) {
			calls <- 1
			return "", errors.New("transient")
		})
		done <- struct{}{}
	}()

	<-calls
	require.Eventually(t, fakeClk.HasWaiters, time.Second, time.Millisecond)
	// Step the override exactly — if OnRetryWait were ignored, the helper
	// would still be waiting for the huge Backoff interval and the next
	// call would not fire.
	fakeClk.Step(override)
	<-calls

	require.Eventually(t, fakeClk.HasWaiters, time.Second, time.Millisecond)
	fakeClk.Step(override)
	<-calls

	<-done
}

func TestDo_OnRetryWaitNonPositiveFallsBackToBackoff(t *testing.T) {
	ctx := context.Background()

	calls := 0
	_, err := Do(ctx, Options[string]{
		MaxAttempts: 3,
		Backoff:     &backoff.ConstantBackOff{Interval: 1 * time.Millisecond},
		OnRetryWait: func(_ string, _ error) time.Duration {
			return 0
		},
	}, func(ctx context.Context) (string, error) {
		calls++
		return "", errors.New("transient")
	})

	require.Error(t, err)
	assert.Equal(t, 3, calls, "OnRetryWait returning 0 must not short-circuit; Backoff still runs")
}

func TestLinearBackOff(t *testing.T) {
	b := &LinearBackOff{Step: 100 * time.Millisecond}

	assert.Equal(t, 100*time.Millisecond, b.NextBackOff())
	assert.Equal(t, 200*time.Millisecond, b.NextBackOff())
	assert.Equal(t, 300*time.Millisecond, b.NextBackOff())

	b.Reset()
	assert.Equal(t, 100*time.Millisecond, b.NextBackOff(), "Reset restarts the sequence")
}
