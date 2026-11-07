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
	if p == nil || p.program == nil || ctx == nil || prepare == nil {
		return nil, ErrInvalidInput
	}

	return EvaluateJSON(ctx, p.limits, func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
		// Install cleanup before preparation so partially initialized hosts also
		// close on errors or Goja interrupt panics. Close precedes output encoding.
		invocation := &AsyncInvocation{}
		defer func() {
			if invocation.Close != nil {
				invocation.Close()
			}
		}()
		if err := prepare(runCtx, vm, invocation); err != nil {
			return nil, err
		}
		if invocation.Params == nil || invocation.Context == nil {
			return nil, ErrInvalidInput
		}
		if err := runCtx.Err(); err != nil {
			return nil, err
		}

		tracker := NewPromiseTracker(vm)
		defer tracker.Close()
		execute, err := p.Initialize(vm)
		if err != nil {
			return nil, err
		}
		if err := tracker.CheckRejections(); err != nil {
			return nil, err
		}
		if err := runCtx.Err(); err != nil {
			return nil, err
		}
		if invocation.Start != nil {
			invocation.Start()
		}

		root, err := callAsync(execute, invocation.Params, invocation.Context)
		if err != nil {
			return nil, err
		}
		for root.State() == goja.PromiseStatePending {
			if err := runCtx.Err(); err != nil {
				return nil, err
			}
			if invocation.Await == nil {
				return nil, ErrEvaluation
			}
			if err := invocation.Await(); err != nil {
				return nil, err
			}
		}
		return tracker.SettledResult(root)
	})
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
