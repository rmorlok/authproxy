package javascript

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

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
