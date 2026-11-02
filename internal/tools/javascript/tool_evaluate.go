package javascript

import (
	"context"
	"reflect"

	"github.com/dop251/goja"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Evaluate runs execute(params, context) in a fresh runtime and converts its
// fulfilled Promise value into detached JSON. The supplied input is copied
// before connector initialization, so helper code can read cfg and the other
// captured aliases. The caller owns argument-schema validation and admission.
//
// This execution path supplies no I/O capabilities. Native Promise chains and
// await work normally, but a pending Promise with no host work cannot make
// progress and returns ErrEvaluation. Rejected entry points and otherwise
// unhandled rejections also return ErrEvaluation without exposing their values.
// The entry point must return a native Promise, even if top-level code replaced
// the async declaration. Output follows Expression.Evaluate's JSON rules.
//
// The policy timeout and caller cancellation cover input copying, initialization,
// promise settlement, and output conversion. No runtime or pending authored
// state is reused; compiled programs may be evaluated concurrently.
func (p *ToolProgram) Evaluate(ctx context.Context, input Input) (common.RawJSON, error) {
	if p == nil || p.program == nil || ctx == nil {
		return nil, ErrInvalidInput
	}

	var result common.RawJSON
	err := withRuntime(ctx, p.limits, func(runCtx context.Context, vm *goja.Runtime) error {
		// Capture conversion helpers before either source program can modify
		// them, then bind the private input needed by connector initialization.
		encoder := newOutputEncoder(vm, p.limits)
		if err := bindInput(runCtx, vm, input, p.limits); err != nil {
			return err
		}

		// Install rejection tracking before any authored initialization. This
		// pure path admits no worker jobs; a future I/O host sets its own cap.
		loop := newEventLoop(runCtx, vm, 0)
		defer loop.close()
		execute, err := p.Initialize(vm)
		if err != nil {
			return err
		}
		value, err := execute(goja.Undefined(), vm.Get("params"), vm.Get("context"))
		if err != nil {
			return runtimeEvaluationError(err)
		}

		// Check the native export type before Export: exporting an arbitrary
		// authored object could traverse getters or allocate an unchecked graph.
		if value == nil || value.ExportType() != reflect.TypeFor[*goja.Promise]() {
			return ErrEvaluation
		}
		settled, err := loop.await(value.Export().(*goja.Promise))
		if err != nil {
			return err
		}
		result, err = encoder.encode(runCtx, settled)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
