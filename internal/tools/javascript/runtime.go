package javascript

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// evaluateResult installs pure Tool result helpers, binds a private input, and
// converts successful content under one runtime deadline. The callback owns
// execution and any HTTP cleanup; no partial result survives a later failure.
// Expressions use evaluateJSON and never receive Tool result helpers.
func evaluateResult(
	ctx context.Context,
	limits Limits,
	input Input,
	run func(context.Context, *goja.Runtime) (goja.Value, error),
) (result *Result, err error) {
	err = withRuntime(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) error {
		encoder, err := newResultEncoder(vm, limits)
		if err != nil {
			return err
		}
		if err := bindInput(runCtx, vm, input, limits); err != nil {
			return err
		}
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

// evaluateJSON binds a private input copy, runs authored code, and converts its
// result under one runtime deadline. The callback owns evaluation and safe error
// classification; callers validate the context, limits, and program beforehand.
// Conversion intrinsics are captured before input binding or authored code can
// change the VM. No output is returned if runtime cleanup observes cancellation
// or any other failure, including one after conversion has finished.
func evaluateJSON(
	ctx context.Context,
	limits Limits,
	input Input,
	run func(context.Context, *goja.Runtime) (goja.Value, error),
) (result common.RawJSON, err error) {
	err = withRuntime(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) error {
		encoder := newOutputEncoder(vm, limits)
		if err := bindInput(runCtx, vm, input, limits); err != nil {
			return err
		}
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

// withRuntime executes work in a fresh, stack-bounded runtime with a joined
// interrupt watcher. Work and all ordinary VM access stay on the calling
// goroutine. It sanitizes runtime failures and gives caller cancellation priority
// over the policy timeout, including cancellation during final conversion.
func withRuntime(
	ctx context.Context,
	limits Limits,
	work func(context.Context, *goja.Runtime) error,
) (err error) {
	runCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	if err := expressionContextError(ctx, runCtx); err != nil {
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
		if contextErr := expressionContextError(ctx, runCtx); contextErr != nil {
			err = contextErr
		}
	}()

	return work(runCtx, vm)
}

// runtimeEvaluationError classifies VM and connector-library errors without
// retaining diagnostics that can contain source or private authored values.
// Cancellation classification belongs to the runtime owner, which knows whether
// the caller or its own policy deadline caused the interruption.
func runtimeEvaluationError(err error) error {
	var stackErr *goja.StackOverflowError
	if errors.As(err, &stackErr) {
		return ErrLimitExceeded
	}
	return ErrEvaluation
}
