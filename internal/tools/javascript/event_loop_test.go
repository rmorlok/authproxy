package javascript

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// loopContext bounds tests so a lost completion fails instead of hanging.
func loopContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// loopPromise evaluates source on the owner and extracts its native promise.
func loopPromise(t *testing.T, vm *goja.Runtime, source string) *goja.Promise {
	t.Helper()
	value, err := vm.RunString(source)
	require.NoError(t, err)
	promise, ok := value.Export().(*goja.Promise)
	require.True(t, ok)
	return promise
}

// TestEventLoopSettledPromises covers microtask draining and quiescent outcomes.
func TestEventLoopSettledPromises(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   int64
		err    error
	}{
		"fulfilled":          {source: `Promise.resolve(4)`, want: 4},
		"microtasks":         {source: `(async () => (await Promise.resolve(4)) + 3)()`, want: 7},
		"root rejected":      {source: `Promise.reject("private reason")`, err: ErrEvaluation},
		"root stuck":         {source: `new Promise(() => {})`, err: ErrEvaluation},
		"detached rejection": {source: `Promise.reject("private reason"); Promise.resolve(4)`, err: ErrEvaluation},
		"handled rejection":  {source: `Promise.reject("private reason").catch(() => {}); Promise.resolve(4)`, want: 4},
		"partially handled":  {source: `Promise.reject("private").catch(() => {}); Promise.reject("another"); Promise.resolve(4)`, err: ErrEvaluation},
		"multiple handlers":  {source: `const rejected = Promise.reject("private"); rejected.catch(() => {}); rejected.catch(() => {}); Promise.resolve(4)`, want: 4},
	} {
		t.Run(name, func(t *testing.T) {
			// Install tracking before authored code creates or rejects any promises.
			vm := goja.New()
			loop := newEventLoop(loopContext(t), vm, 0)
			defer loop.close()
			root := loopPromise(t, vm, test.source)

			// Only a fulfilled root with no outstanding unhandled failure succeeds.
			value, err := loop.await(root)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
				require.Nil(t, value)
				require.NotContains(t, err.Error(), "private")
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, value.ToInteger())
			}
		})
	}
}

// TestEventLoopConcurrentCompletions settles native promises only on the owner.
func TestEventLoopConcurrentCompletions(t *testing.T) {
	// Workers may finish in either order, while Promise.all preserves its inputs.
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 2)
	defer loop.close()
	for index, name := range []string{"first", "second"} {
		promise, resolve, _ := vm.NewPromise()
		require.NoError(t, vm.Set(name, promise))
		require.NoError(t, loop.start(func(context.Context) (any, error) {
			return int64(index + 1), nil
		}, func(value any, err error) error {
			require.NoError(t, err)
			return resolve(value)
		}))
	}
	root := loopPromise(t, vm, `(async () => (await Promise.all([first, second])).join(","))()`)

	// Resolver calls drain their JavaScript reactions before the loop continues.
	value, err := loop.await(root)
	require.NoError(t, err)
	require.Equal(t, "1,2", value.String())
	require.Empty(t, loop.pending)
}

// TestEventLoopNestedAdmission frees capacity before a completion starts more work.
func TestEventLoopNestedAdmission(t *testing.T) {
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 1)
	defer loop.close()
	root, resolve, _ := vm.NewPromise()

	// A completion can admit another operation without a second pending slot.
	require.NoError(t, loop.start(func(context.Context) (any, error) {
		return int64(6), nil
	}, func(value any, err error) error {
		require.NoError(t, err)
		return loop.start(func(context.Context) (any, error) {
			return value.(int64) + 1, nil
		}, func(value any, err error) error {
			require.NoError(t, err)
			return resolve(value)
		})
	}))

	value, err := loop.await(root)
	require.NoError(t, err)
	require.Equal(t, int64(7), value.ToInteger())
}

// TestEventLoopDrainsUnawaitedJobs ensures success does not leave admitted work.
func TestEventLoopDrainsUnawaitedJobs(t *testing.T) {
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 1)
	defer loop.close()
	root := loopPromise(t, vm, `Promise.resolve(9)`)
	settled := false

	// The root is already fulfilled, but its unawaited worker must still settle.
	require.NoError(t, loop.start(func(context.Context) (any, error) {
		return nil, nil
	}, func(any, error) error {
		settled = true
		return nil
	}))
	value, err := loop.await(root)
	require.NoError(t, err)
	require.Equal(t, int64(9), value.ToInteger())
	require.True(t, settled)
}

// TestEventLoopLateRejectionHandler permits handlers installed by host completion.
func TestEventLoopLateRejectionHandler(t *testing.T) {
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 1)
	defer loop.close()
	root := loopPromise(t, vm, `globalThis.rejected = Promise.reject("private"); Promise.resolve(5)`)

	// Quiescence is checked after pending completions can attach a catch handler.
	require.NoError(t, loop.start(func(context.Context) (any, error) {
		return nil, nil
	}, func(any, error) error {
		_, err := vm.RunString(`rejected.catch(() => {})`)
		return err
	}))
	value, err := loop.await(root)
	require.NoError(t, err)
	require.Equal(t, int64(5), value.ToInteger())
}

// TestEventLoopAdmissionLimits rejects invalid or excessive jobs without launch.
func TestEventLoopAdmissionLimits(t *testing.T) {
	for _, maxPending := range []int{0, 1} {
		vm := goja.New()
		loop := newEventLoop(loopContext(t), vm, maxPending)
		defer loop.close()
		work := func(ctx context.Context) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		settle := func(any, error) error { return nil }

		// Invalid callbacks never consume capacity; zero is a capability-free cap.
		require.ErrorIs(t, loop.start(nil, settle), ErrInvalidInput)
		require.ErrorIs(t, loop.start(work, nil), ErrInvalidInput)
		if maxPending == 1 {
			require.NoError(t, loop.start(work, settle))
		}
		require.ErrorIs(t, loop.start(work, settle), ErrLimitExceeded)
		require.Len(t, loop.pending, maxPending)

		// Closing cancels existing jobs and prevents all later admission.
		loop.close()
		loop.close()
		require.ErrorIs(t, loop.start(work, settle), ErrInvalidInput)
	}
}

// TestEventLoopCancellationSkipsSettlement gives cancellation priority over work.
func TestEventLoopCancellationSkipsSettlement(t *testing.T) {
	ctx, cancel := context.WithCancel(loopContext(t))
	defer cancel()
	vm := goja.New()
	loop := newEventLoop(ctx, vm, 1)
	defer loop.close()
	root, _, _ := vm.NewPromise()
	settled := false
	finished := make(chan struct{})

	// Cancel while a worker is active, then let it return a result concurrently.
	require.NoError(t, loop.start(func(ctx context.Context) (any, error) {
		cancel()
		<-ctx.Done()
		close(finished)
		return "late private value", nil
	}, func(any, error) error {
		settled = true
		return nil
	}))
	value, err := loop.await(root)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, value)
	require.False(t, settled)
	<-finished
	require.ErrorIs(t, loop.start(func(context.Context) (any, error) {
		return nil, nil
	}, func(any, error) error { return nil }), context.Canceled)
}

// TestEventLoopCloseDiscardsLateWork never waits for uncooperative worker code.
func TestEventLoopCloseDiscardsLateWork(t *testing.T) {
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 1)
	defer loop.close()
	release, finished := make(chan struct{}), make(chan struct{})
	settled := false

	// Simulate native work that does not observe cancellation until it returns.
	require.NoError(t, loop.start(func(context.Context) (any, error) {
		<-release
		close(finished)
		return "late private value", nil
	}, func(any, error) error {
		settled = true
		return nil
	}))
	loop.close()
	close(release)
	<-finished

	// The owner dropped the only callback; no late data can reach the runtime.
	require.Nil(t, loop.pending)
	require.False(t, settled)
	require.ErrorIs(t, loop.start(nil, nil), ErrInvalidInput)
}

// TestEventLoopSettlementFailures sanitizes errors from owner-side callbacks.
func TestEventLoopSettlementFailures(t *testing.T) {
	for name, callbackError := range map[string]error{
		"private error": errors.New("private callback error"),
		"stack limit":   &goja.StackOverflowError{},
		"host limit":    ErrLimitExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			vm := goja.New()
			loop := newEventLoop(loopContext(t), vm, 1)
			defer loop.close()
			root, _, _ := vm.NewPromise()

			// Native promise resolvers return uncatchable errors to their caller.
			require.NoError(t, loop.start(func(context.Context) (any, error) {
				return nil, nil
			}, func(any, error) error { return callbackError }))
			value, err := loop.await(root)
			if name == "stack limit" || name == "host limit" {
				require.ErrorIs(t, err, ErrLimitExceeded)
			} else {
				require.ErrorIs(t, err, ErrEvaluation)
			}
			require.Nil(t, value)
			require.NotContains(t, err.Error(), "private")
		})
	}
}

// TestEventLoopWorkerPanic becomes a sanitized completion instead of a Go crash.
func TestEventLoopWorkerPanic(t *testing.T) {
	vm := goja.New()
	loop := newEventLoop(loopContext(t), vm, 1)
	defer loop.close()
	root, _, reject := vm.NewPromise()
	cleaned := false

	// A panicking worker runs its own cleanup and returns no private panic value.
	require.NoError(t, loop.start(func(context.Context) (any, error) {
		defer func() { cleaned = true }()
		panic("private worker panic")
	}, func(value any, err error) error {
		require.Nil(t, value)
		require.ErrorIs(t, err, ErrEvaluation)
		require.NotContains(t, err.Error(), "private")
		return reject(err.Error())
	}))
	value, err := loop.await(root)
	require.Nil(t, value)
	require.ErrorIs(t, err, ErrEvaluation)
	require.True(t, cleaned)
}

// TestEventLoopRootRejectionDoesNotDrain abandons pending callbacks on failure.
func TestEventLoopRootRejectionDoesNotDrain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	vm := goja.New()
	loop := newEventLoop(ctx, vm, 1)
	defer loop.close()
	root := loopPromise(t, vm, `Promise.reject("private")`)
	finished := make(chan struct{})

	// The worker cannot finish until close cancels it after the root failure.
	require.NoError(t, loop.start(func(ctx context.Context) (any, error) {
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	}, func(any, error) error {
		t.Fatal("a failed invocation must not settle more callbacks")
		return nil
	}))
	value, err := loop.await(root)
	require.ErrorIs(t, err, ErrEvaluation)
	require.Nil(t, value)
	loop.close()
	<-finished
}
