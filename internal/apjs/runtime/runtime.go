package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

// JSONOutputOptions selects explicit serialization behavior for a trusted
// adapter. The zero value keeps the strict JSON data contract; options do not
// relax prototype, accessor, cycle, or structural-limit validation.
type JSONOutputOptions struct {
	// OmitUndefinedProperties omits own enumerable object data properties whose
	// value is undefined, recursively, including objects contained in arrays.
	// Inspected keys still consume the traversal budget, but omitted properties
	// consume no output bytes. This takes precedence over UndefinedAsNull for
	// object properties; array elements and roots are unaffected by omission.
	OmitUndefinedProperties bool

	// UndefinedAsNull encodes explicit JavaScript undefined as JSON null,
	// including roots, object data properties, and dense array elements. The
	// replacement consumes the same byte, node, and depth allowances as null.
	// Missing array elements, accessors, and nil Go values remain invalid. This
	// is an adapter compatibility policy, not JavaScript coercion or toJSON.
	UndefinedAsNull bool
}

// EvaluateJSON runs a trusted host adapter in a fresh bounded VM and converts
// its value into detached JSON before ending the runtime. Conversion intrinsics
// are captured before the callback can bind input or execute authored code.
// Unsupported output or any later cancellation discards the entire result.
//
// The callback owns input binding and program execution. It must bound input
// under limits, obey runCtx, keep VM access on its calling goroutine, and retain
// no VM values after returning. It returns safe host errors unchanged and uses
// EvaluationError for authored exceptions or compile/run errors. Only this
// callback may return a Goja value to the encoder; callers receive JSON bytes.
func EvaluateJSON(
	ctx context.Context,
	limits Limits,
	run func(context.Context, *goja.Runtime) (goja.Value, error),
) (json.RawMessage, error) {
	return EvaluateJSONWithOptions(ctx, limits, JSONOutputOptions{}, run)
}

// EvaluateJSONWithOptions applies an adapter's explicit output policy while
// retaining EvaluateJSON's bounded lifecycle and callback ownership contract.
// Reflection intrinsics are captured before authored code runs; options never
// enable getters, proxy traps, toJSON methods, or JavaScript coercion.
func EvaluateJSONWithOptions(
	ctx context.Context,
	limits Limits,
	options JSONOutputOptions,
	run func(context.Context, *goja.Runtime) (goja.Value, error),
) (result json.RawMessage, err error) {
	if run == nil {
		return nil, ErrInvalidInput
	}
	err = Run(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) error {
		encoder := newOutputEncoder(vm, limits)
		encoder.options = options
		value, err := run(runCtx, vm)
		if err != nil {
			return err
		}
		result, err = encoder.encode(runCtx, value)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Run executes trusted host work in a fresh stack-bounded runtime with a joined
// interrupt watcher. It is also used for initialization validation that returns
// no data. Work and ordinary VM access stay on the calling goroutine. Only the
// thread-safe Interrupt method runs elsewhere; no VM or value may escape work.
// Pure utilities are always installed. Managed input/output conversion, utilities,
// and HTTP work inherit one cumulative budget through runCtx. Nested runs also
// consume the enclosing execution's allowance. Exhaustion interrupts authored
// execution; ordinary malformed helper input remains catchable.
//
// The callback must honor runCtx during Go work, enforce any source/input bounds,
// and convert authored errors with EvaluationError. Its safe host classifications
// are preserved. Caller cancellation takes priority over the policy timeout,
// including during final cleanup. Unknown Go panics are rethrown, not hidden.
func Run(
	ctx context.Context,
	limits Limits,
	work func(context.Context, *goja.Runtime) error,
) (err error) {
	if ctx == nil || work == nil {
		return ErrInvalidInput
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	budgetCtx, err := WithExecutionBudget(runCtx, limits)
	if err != nil {
		// Policy expiry can precede budget admission or watcher installation.
		// Preserve its limit classification and the caller's cancellation priority.
		if contextErr := contextError(ctx, runCtx); contextErr != nil {
			return contextErr
		}
		return err
	}
	runCtx = budgetCtx
	budget := executionBudgetFromContext(runCtx)
	if err := contextError(ctx, runCtx); err != nil {
		return err
	}

	vm := goja.New()
	vm.SetParserOptions(parser.WithDisableSourceMaps)
	vm.SetMaxCallStackSize(limits.MaxStackDepth)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-runCtx.Done():
			vm.Interrupt(runCtx.Err())
		case <-budget.state.done:
			vm.Interrupt(ErrLimitExceeded)
		case <-stop:
		}
	}()
	defer func() {
		// Join before discarding the VM so no late interrupt outlives this work.
		close(stop)
		<-stopped
		if recovered := recover(); recovered != nil {
			switch recovered.(type) {
			case *goja.InterruptedError:
				err = ErrEvaluation
			case *goja.StackOverflowError:
				err = ErrLimitExceeded
			default:
				panic(recovered)
			}
		}
		if budgetErr := budget.err(); budgetErr != nil {
			err = budgetErr
		}
		if contextErr := contextError(ctx, runCtx); contextErr != nil {
			err = contextErr
		}
	}()

	_, err = installUtilities(runCtx, vm)
	if err != nil {
		return err
	}
	return work(runCtx, vm)
}

// EvaluationError classifies an authored compile/run or library failure without
// retaining source, thrown values, or private diagnostics. Nil stays nil; stack
// exhaustion is a limit failure; other errors are generic evaluation failures.
// Run applies caller/policy cancellation precedence after the callback returns.
// Do not pass safe host classifications through this function: return them as-is.
func EvaluationError(err error) error {
	if err == nil {
		return nil
	}
	var stackErr *goja.StackOverflowError
	if errors.As(err, &stackErr) {
		return ErrLimitExceeded
	}
	return ErrEvaluation
}

// contextError distinguishes caller cancellation from the runtime's own limit.
// The caller's error wins when both contexts are done. runCtx is private and
// independently canceled only by policy expiry until runtime cleanup.
func contextError(ctx, runCtx context.Context) error {
	if runCtx.Err() != nil {
		// Read the caller after observing the child: inherited cancellation
		// must already be visible there, even if it arrived during this check.
		if err := ctx.Err(); err != nil {
			return err
		}
		return ErrLimitExceeded
	}

	// The caller can be canceled before cancellation reaches its child.
	return ctx.Err()
}
