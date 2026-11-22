package javascript

import (
	"context"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// Evaluate runs execute(params, context) in a fresh runtime and converts its
// settled Promise value into a detached Tool Result. The supplied input is copied
// before connector initialization; helpers called by execute can read cfg and
// the other captured aliases. The caller owns schema validation and admission.
//
// This execution path supplies no I/O capabilities and context.connections is
// empty. Native Promise chains and await work normally, but a pending Promise with
// no host work cannot make progress and returns apjsruntime.ErrEvaluation. Rejected
// entry points return apjsruntime.ErrEvaluation without exposing their values,
// except genuine ToolError rejections: these return Result.Error and nil Go error.
// Unrelated unhandled rejections still fail, even alongside a ToolError. The entry point must
// return a native Promise, even if top-level code replaced the async declaration.
// Ordinary JSON becomes structured content; strings also produce a text block.
// Only toolResult handles opt into explicit rich content.
//
// The policy timeout and caller cancellation cover input copying, initialization,
// promise settlement, and output conversion. No runtime or pending authored
// state is reused; compiled programs may be evaluated concurrently.
func (p *ToolProgram) Evaluate(ctx context.Context, input Input) (*Result, error) {
	if p == nil || p.program == nil || ctx == nil {
		return nil, apjsruntime.ErrInvalidInput
	}

	return p.program.Evaluate(
		ctx,
		func(
			runCtx context.Context,
			vm *goja.Runtime,
			invocation *apjsruntime.AsyncInvocation,
		) error {
			return p.prepareInvocation(
				runCtx,
				vm,
				invocation,
				input,
				nil, // host
				nil, // session
			)
		},
	)
}
