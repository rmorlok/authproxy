package javascript

import (
	"context"
	"errors"

	"github.com/dop251/goja"
)

// eventLoop returns asynchronous host completions to the one goroutine that
// owns vm. Only start's work function runs elsewhere: it receives cancellation
// and returns detached Go data, never a Goja value or an open resource. Workers
// must close their I/O themselves, including after cancellation.
//
// All methods, promise tracking, and settlement callbacks belong to the owner
// goroutine. The caller must defer close, including when initialization fails.
// Goja itself drains JavaScript microtasks whenever a VM call returns; this loop
// supplies external completions rather than a second JavaScript job queue.
type eventLoop struct {
	// vm remains on the owner goroutine; workers never capture or access it.
	vm *goja.Runtime

	// ctx cancels worker operations and completion delivery together.
	ctx    context.Context
	cancel context.CancelFunc

	// maxPending bounds admitted workers whose completions have not settled.
	// Zero disables host jobs, as used by capability-free Tool evaluation.
	maxPending int

	// completed transfers plain Go data without buffering completed payloads.
	completed chan eventCompletion

	// pending holds VM-owning callbacks exclusively on the owner goroutine.
	pending map[uint64]func(any, error) error

	// nextID gives each admission an identity independent of completion order.
	nextID uint64

	// unhandled counts rejected promises until a later handler claims them.
	// Tracking starts before authored code in a fresh VM; Goja reports each
	// rejection once and its first later handler once. Counting avoids retaining
	// private rejection values through an additional map of promise references.
	unhandled int

	// closed prevents new work or settlement after the owner ends the loop.
	closed bool
}

// eventCompletion contains only a worker's detached result and job identity.
// Promise resolvers and other runtime-owned callbacks stay in pending.
type eventCompletion struct {
	// id associates this result with its owner-side settlement callback.
	id uint64

	// value transfers worker-owned Go data; the worker must not mutate it again.
	value any

	// err is private worker failure data for the callback to classify safely.
	err error
}

// newEventLoop installs rejection tracking before any authored initialization.
// Its private callers provide a nonnil context/runtime and nonnegative job cap.
// The cap bounds concurrency, not total host requests; the host applies its own
// cumulative invocation budget before admitting each operation.
func newEventLoop(ctx context.Context, vm *goja.Runtime, maxPending int) *eventLoop {
	ctx, cancel := context.WithCancel(ctx)
	loop := &eventLoop{
		vm:         vm,
		ctx:        ctx,
		cancel:     cancel,
		maxPending: maxPending,
		completed:  make(chan eventCompletion),
		pending:    make(map[uint64]func(any, error) error),
	}
	vm.SetPromiseRejectionTracker(loop.trackRejection)
	return loop
}

// trackRejection records rejection state without inspecting private reasons.
// Goja calls it synchronously on the VM owner, including during settlement.
func (l *eventLoop) trackRejection(_ *goja.Promise, operation goja.PromiseRejectionOperation) {
	switch operation {
	case goja.PromiseRejectionHandle:
		l.unhandled--
	case goja.PromiseRejectionReject:
		l.unhandled++
	}
}

// start admits a worker without letting it access vm or its settlement callback.
// Work must return detached data and own all resource cleanup; settle translates
// that data or a sanitized failure on the VM owner and propagates any error from
// Goja's promise resolver. Nil callbacks, cancellation, and a full job allowance
// reject admission before launching a goroutine. Calling start from a settlement
// callback is supported and consumes the newly available pending-job allowance.
func (l *eventLoop) start(
	work func(context.Context) (any, error),
	settle func(any, error) error,
) error {
	if l.closed || work == nil || settle == nil {
		return ErrInvalidInput
	}
	if err := l.ctx.Err(); err != nil {
		return err
	}
	if len(l.pending) >= l.maxPending {
		return ErrLimitExceeded
	}

	l.nextID++
	id := l.nextID
	l.pending[id] = settle

	// Capture only worker-safe values, not the loop or its VM-owned callback map.
	ctx, completed := l.ctx, l.completed
	go func() {
		value, err := runEventWork(ctx, work)
		select {
		case completed <- eventCompletion{id: id, value: value, err: err}:
		case <-ctx.Done():
			// Cancellation discards late data without touching the runtime.
		}
	}()
	return nil
}

// runEventWork contains a worker panic without carrying its value or stack back
// into the VM. The worker still owns ordinary deferred cleanup of its resources.
func runEventWork(ctx context.Context, work func(context.Context) (any, error)) (value any, err error) {
	defer func() {
		if recover() != nil {
			value, err = nil, ErrEvaluation
		}
	}()
	return work(ctx)
}

// await settles admitted host work and returns a fulfilled root's value only
// after every completion has run and no rejected promise remains unhandled.
// A rejected root fails immediately. A pending root with no remaining host work
// cannot make progress and fails rather than waiting until the timeout. A caller
// supplies a genuine promise from this runtime and defers close on every path.
//
// Successful completion drains jobs even when authored code did not await them.
// Failure/cancellation may leave Go workers running until they observe their
// context; close discards their callbacks, and no late result can access the VM.
func (l *eventLoop) await(root *goja.Promise) (goja.Value, error) {
	if l.closed || root == nil {
		return nil, ErrInvalidInput
	}
	for {
		if err := l.ctx.Err(); err != nil {
			return nil, err
		}
		if root.State() == goja.PromiseStateRejected {
			return nil, ErrEvaluation
		}
		if len(l.pending) == 0 {
			if root.State() != goja.PromiseStateFulfilled || l.unhandled != 0 {
				return nil, ErrEvaluation
			}
			return root.Result(), nil
		}

		select {
		case <-l.ctx.Done():
			return nil, l.ctx.Err()
		case completion := <-l.completed:
			// Cancellation wins even when a result was simultaneously ready.
			if err := l.ctx.Err(); err != nil {
				return nil, err
			}
			settle := l.pending[completion.id]
			delete(l.pending, completion.id)
			if err := settle(completion.value, completion.err); err != nil {
				return nil, eventSettlementError(err)
			}
		}
	}
}

// eventSettlementError preserves fixed host classifications without forwarding
// wrapped diagnostics; arbitrary callback and Goja errors use runtime categories.
func eventSettlementError(err error) error {
	for _, safe := range []error{ErrLimitExceeded, ErrInvalidInput, ErrInvalidOutput, ErrEvaluation} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return runtimeEvaluationError(err)
}

// close cancels workers, drops VM-owned callbacks, and removes rejection tracking.
// It is idempotent but owner-goroutine-only, and does not wait for uncooperative
// Go operations. Hosts must retain conservative execution evidence after early
// return: closing is not proof that outstanding I/O finished or was never sent.
// Workers completing later discard plain data and cannot invoke authored code.
func (l *eventLoop) close() {
	if l.closed {
		return
	}
	l.closed = true
	l.cancel()
	l.pending = nil
	l.unhandled = 0
	l.vm.SetPromiseRejectionTracker(nil)
}
