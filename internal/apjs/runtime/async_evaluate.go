package runtime

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/dop251/goja"
)

// AsyncInvocation supplies one execution's native arguments and optional host
// lifecycle. The trusted preparation callback fills it in on the VM goroutine;
// neither the invocation nor its values may escape evaluation. Callbacks must
// honor the preparation context and must not access the VM from other goroutines.
type AsyncInvocation struct {
	// Params is the first execute argument. It must be a value from this VM or a
	// primitive such as goja.Undefined(), never a reflected live Go object.
	Params goja.Value

	// Context is the second execute argument, with the same ownership as Params.
	// Adapters decide its shape and any global aliases; the runtime adds none.
	Context goja.Value

	// Start enables optional host capabilities only after initialization and its
	// Promise jobs succeed. Hosts must refuse I/O before this callback runs.
	Start func()

	// Await processes one host completion on the VM goroutine while execute is
	// pending. It may settle Promises and thereby run authored callbacks. Return
	// ErrEvaluation when no work can progress, or another safe host error on
	// failure. A nil callback means pending execution cannot make progress.
	Await func() error

	// Close cancels outstanding host work on every exit, including preparation
	// failure or panic, before output encoding. Register it immediately after
	// acquiring resources, before another VM call can be interrupted. It must
	// not wait for uncooperative workers or run authored code. Nil needs no cleanup.
	Close func()
}

// Evaluate prepares a fresh VM, initializes the program, invokes execute with
// the supplied arguments, and converts its fulfilled Promise value to detached
// JSON. Preparation, execution, host completion, and encoding share one deadline.
// A rejected or unprogressable root, or an unhandled rejection at initialization
// or final settlement, fails safely without exposing private rejection values.
//
// prepare is required trusted adapter code. It binds bounded native JSON (for
// example with JSONDecoder), fills both arguments, and optionally installs host
// capabilities. Use goja.Undefined() for an absent argument. It returns only safe
// errors and must honor runCtx during Go work. The encoder captures pristine
// intrinsics before preparation. No VM values reach the caller.
//
// Once the root settles, remaining host completions are discarded and Close
// cancels outstanding work. Rejections are checked at final settlement rather
// than after each completion, allowing handlers added after an awaited operation.
// Policy expiry returns ErrLimitExceeded; caller cancellation takes precedence
// and retains its context error. Any failure discards all output.
func (p *AsyncProgram) Evaluate(
	ctx context.Context,
	prepare func(context.Context, *goja.Runtime, *AsyncInvocation) error,
) (json.RawMessage, error) {
	return evaluateAsync(ctx, p, prepare, func(context.Context, *goja.Runtime) (ResultInterpreter[json.RawMessage], error) {
		return ResultInterpreter[json.RawMessage]{
			Fulfilled: func(_ context.Context, value goja.Value, encoder *ResultEncoder) (json.RawMessage, error) {
				return encoder.Encode(value)
			},
		}, nil
	})
}

// evaluate owns one async invocation's initialization, host lifecycle, and
// rejection tracking. install runs before preparation and authored code;
// interpret runs after host cleanup, on the same owner and under the same
// deadline. Neither callback may retain VM state after evaluation ends.
func (p *AsyncProgram) evaluate(
	runCtx context.Context,
	vm *goja.Runtime,
	prepare func(context.Context, *goja.Runtime, *AsyncInvocation) error,
	install func() error,
	interpret func(*goja.Promise) error,
) error {
	tracker := NewPromiseTracker(vm)
	defer tracker.Close()

	// Register cleanup before installing helpers or preparing hosts, including
	// partial preparation failure and interrupts during subsequent VM calls.
	invocation := &AsyncInvocation{}
	closeHost := func() {
		if close := invocation.Close; close != nil {
			invocation.Close = nil
			close()
		}
	}
	defer closeHost()
	if err := install(); err != nil {
		return err
	}
	if err := prepare(runCtx, vm, invocation); err != nil {
		return err
	}
	if invocation.Params == nil || invocation.Context == nil {
		return ErrInvalidInput
	}
	if err := runCtx.Err(); err != nil {
		return err
	}

	execute, err := p.Initialize(vm)
	if err != nil {
		return err
	}
	if err := tracker.CheckRejections(); err != nil {
		return err
	}
	if err := runCtx.Err(); err != nil {
		return err
	}
	if invocation.Start != nil {
		invocation.Start()
	}

	root, err := callAsync(execute, invocation.Params, invocation.Context)
	if err != nil {
		return err
	}
	for root.State() == goja.PromiseStatePending {
		if err := runCtx.Err(); err != nil {
			return err
		}
		if invocation.Await == nil {
			return ErrEvaluation
		}
		if err := invocation.Await(); err != nil {
			return err
		}
	}

	// Root settlement ends HTTP admission before any result conversion. Check
	// unrelated failures both before interpretation and after trusted callbacks
	// that might themselves drain microtasks. Only the exact root is exempt.
	closeHost()
	if err := tracker.checkRejectionsExcept(root); err != nil {
		return err
	}
	if err := runCtx.Err(); err != nil {
		return err
	}
	resultErr := interpret(root)
	if err := tracker.checkRejectionsExcept(root); err != nil {
		return err
	}
	return resultErr
}

// callAsync invokes the entry point and requires a native Promise. Checking its
// export type first avoids traversing arbitrary getters or exporting an unchecked
// object graph when authored initialization replaces the entry point.
func callAsync(execute goja.Callable, params, contextValue goja.Value) (*goja.Promise, error) {
	value, err := execute(goja.Undefined(), params, contextValue)
	if err != nil {
		return nil, EvaluationError(err)
	}
	if value == nil || value.ExportType() != reflect.TypeFor[*goja.Promise]() {
		return nil, ErrEvaluation
	}
	return value.Export().(*goja.Promise), nil
}
