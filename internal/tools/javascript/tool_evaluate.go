package javascript

import (
	"context"
	"reflect"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Evaluate runs execute(params, context) in a fresh runtime and converts its
// fulfilled Promise value into detached JSON. The supplied input is copied
// before connector initialization; helpers called by execute can read cfg and
// the other captured aliases. The caller owns schema validation and admission.
//
// This execution path supplies no I/O capabilities. Native Promise chains and
// await work normally, but a pending Promise with no host work cannot make
// progress and returns apjsruntime.ErrEvaluation. Rejected entry points and otherwise
// unhandled rejections also return apjsruntime.ErrEvaluation without exposing their values.
// The entry point must return a native Promise, even if top-level code replaced
// the async declaration. Output follows Expression.Evaluate's JSON rules.
//
// The policy timeout and caller cancellation cover input copying, initialization,
// promise settlement, and output conversion. No runtime or pending authored
// state is reused; compiled programs may be evaluated concurrently.
func (p *ToolProgram) Evaluate(ctx context.Context, input Input) (common.RawJSON, error) {
	if p == nil || p.program == nil || ctx == nil {
		return nil, apjsruntime.ErrInvalidInput
	}

	return evaluateJSON(ctx, p.limits, input, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		// Match publication validation by tracking initialization as well as
		// the entry point. Goja drains Promise microtasks before calls return.
		tracker := apjsruntime.NewPromiseTracker(vm)
		defer tracker.Close()

		execute, err := p.Initialize(vm)
		if err != nil {
			return nil, err
		}

		if err := tracker.CheckRejections(); err != nil {
			return nil, err
		}

		root, err := callTool(vm, execute)
		if err != nil {
			return nil, err
		}
		return tracker.SettledResult(root)
	})
}

// callTool invokes the initialized entry point and requires a native Promise.
// Checking ExportType first avoids traversing an arbitrary returned object's
// getters or allocating an unchecked graph while exporting it.
func callTool(vm *goja.Runtime, execute goja.Callable) (*goja.Promise, error) {
	value, err := execute(goja.Undefined(), vm.Get("params"), vm.Get("context"))
	if err != nil {
		return nil, runtimeEvaluationError(err)
	}
	if value == nil || value.ExportType() != reflect.TypeFor[*goja.Promise]() {
		return nil, apjsruntime.ErrEvaluation
	}
	return value.Export().(*goja.Promise), nil
}
