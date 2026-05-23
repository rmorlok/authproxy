// Package retry provides a small generic helper for retrying operations
// with a caller-supplied backoff strategy and retry classifier.
//
// The package wraps a backoff strategy from cenkalti/backoff/v5 with three
// things the bare library does not give us:
//
//   - Per-callsite retry classification keyed on the operation's *value*
//     (not just its error), so an OAuth callsite can retry "5xx or err"
//     while a 429-aware callsite can retry only "status == 429".
//   - A returned attempt count so failure events can distinguish
//     "exhausted budget" from "single non-retryable failure" — load-bearing
//     for the structured failure events emitted across the codebase.
//   - Sleeps routed through apctx.GetClock(ctx) so tests using a fake clock
//     don't have to wait on wall-clock time.
package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/rmorlok/authproxy/internal/apctx"
)

// Result is what a successful or exhausted retry returns. Attempts is the
// number of operation invocations actually made: 1 on first-try success,
// MaxAttempts on exhaustion, and the count of completed calls on ctx
// cancellation during backoff.
type Result[T any] struct {
	Value    T
	Attempts int
}

// Classifier decides whether a given (value, err) outcome should be retried.
// Return true to retry, false to terminate the loop and return the outcome
// to the caller. The classifier is called after every operation invocation,
// including the last one in the budget — implementations should not assume
// they are only called when retries remain.
type Classifier[T any] func(value T, err error) bool

// Options configures a single Do call.
type Options[T any] struct {
	// MaxAttempts is the total number of operation invocations allowed,
	// including the first. Must be >= 1.
	MaxAttempts int

	// Backoff is the strategy used to compute the wait between attempts.
	// NextBackOff is called once per gap, so for MaxAttempts=N it is
	// called at most N-1 times. If nil, ConstantBackOff(0) is used.
	Backoff backoff.BackOff

	// Classify decides retryability per attempt. If nil, retries on any
	// non-nil error (the most common case).
	Classify Classifier[T]

	// OnRetry, if non-nil, fires after each *retryable* failure that is
	// not the last attempt in the budget — i.e. exactly when a retry is
	// about to be scheduled. Use it to emit per-attempt structured logs
	// without coupling those log strings into this package.
	OnRetry func(attempt int, value T, err error)
}

// Do runs op until it produces a non-retryable outcome, the attempt budget
// is exhausted, or ctx is cancelled. The Classifier decides retryability;
// the Backoff controls inter-attempt waits.
//
// Returned err is:
//   - nil on success.
//   - ctx.Err() if ctx is cancelled before or during a backoff sleep.
//   - The last op error otherwise (including when the last attempt
//     returned a retryable outcome and the budget is gone).
//
// Result.Value carries the last op return value regardless of err, so
// callers can inspect e.g. a final 5xx response on exhaustion.
func Do[T any](
	ctx context.Context,
	opts Options[T],
	op func(ctx context.Context) (T, error),
) (Result[T], error) {
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 1
	}
	if opts.Backoff == nil {
		opts.Backoff = &backoff.ConstantBackOff{Interval: 0}
	}
	classify := opts.Classify
	if classify == nil {
		classify = func(_ T, err error) bool { return err != nil }
	}

	opts.Backoff.Reset()
	clk := apctx.GetClock(ctx)

	var (
		lastVal T
		lastErr error
		attempt int
	)

	for attempt = 1; attempt <= opts.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Result[T]{Value: lastVal, Attempts: attempt - 1}, err
		}

		val, err := op(ctx)
		lastVal, lastErr = val, err

		if !classify(val, err) {
			return Result[T]{Value: val, Attempts: attempt}, err
		}

		if attempt == opts.MaxAttempts {
			break
		}

		if opts.OnRetry != nil {
			opts.OnRetry(attempt, val, err)
		}

		wait := opts.Backoff.NextBackOff()
		if wait == backoff.Stop {
			break
		}

		select {
		case <-ctx.Done():
			return Result[T]{Value: val, Attempts: attempt}, ctx.Err()
		case <-clk.After(wait):
		}
	}

	return Result[T]{Value: lastVal, Attempts: attempt}, lastErr
}

// LinearBackOff returns Step, 2*Step, 3*Step, … from successive
// NextBackOff() calls. cenkalti/backoff/v5 only ships Constant and
// Exponential strategies, so we define our own — the OAuth callsites and
// the disconnect revoke loop all use linear backoff in production.
type LinearBackOff struct {
	Step time.Duration

	n int
}

func (b *LinearBackOff) NextBackOff() time.Duration {
	b.n++
	return b.Step * time.Duration(b.n)
}

func (b *LinearBackOff) Reset() {
	b.n = 0
}
