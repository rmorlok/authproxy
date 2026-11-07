package javascript

import (
	"context"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// evaluateJSON adapts a captured Tool input to the shared bounded runtime.
// Input binding, authored execution, and generic output conversion remain under
// one deadline; the callback owns execution and safe error classification.
func evaluateJSON(
	ctx context.Context,
	limits Limits,
	input Input,
	run func(context.Context, *goja.Runtime) (goja.Value, error),
) (common.RawJSON, error) {
	result, err := apjsruntime.EvaluateJSON(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
		if err := bindInput(runCtx, vm, input, limits); err != nil {
			return nil, err
		}
		return run(runCtx, vm)
	})
	return common.RawJSON(result), err
}

// withRuntime delegates no-result initialization validation to the shared
// lifecycle. Tool compilation and host installation remain the caller's policy.
func withRuntime(ctx context.Context, limits Limits, work func(context.Context, *goja.Runtime) error) error {
	return apjsruntime.Run(ctx, limits, work)
}

// runtimeEvaluationError preserves the shared safe classifications for authored
// failures while Tool-specific hosts continue to use this local adapter.
func runtimeEvaluationError(err error) error {
	return apjsruntime.EvaluationError(err)
}
