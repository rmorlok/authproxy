package javascript

import (
	"context"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Evaluate runs execute(params, context) in a fresh runtime and converts its
// fulfilled Promise value into detached JSON. The supplied input is copied
// before connector initialization; helpers called by execute can read cfg and
// the other captured aliases. The caller owns schema validation and admission.
//
// This execution path supplies no I/O capabilities and context.connections is
// empty. Native Promise chains and await work normally, but a pending Promise with
// no host work cannot make progress and returns apjsruntime.ErrEvaluation. Rejected entry points and otherwise
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

	result, err := p.program.Evaluate(ctx, func(runCtx context.Context, vm *goja.Runtime, invocation *apjsruntime.AsyncInvocation) error {
		return p.prepareInvocation(runCtx, vm, invocation, input, nil, nil)
	})
	return common.RawJSON(result), err
}
