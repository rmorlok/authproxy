// Package javascript evaluates trusted async Tool scripts and pure expressions
// with native JavaScript copies of captured data. I/O is available only when a
// Tool caller explicitly supplies a captured connection's HTTP session.
package javascript

import (
	"context"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/tools/render"
)

// Limits is the shared execution and JSON conversion policy. Tool callers still
// resolve operator ceilings and authored caps before compiling their programs.
type Limits = apjsruntime.Limits

// Input supplies a captured safe invocation snapshot and optional response data.
// Neither field exposes a connection handle, credentials, or arbitrary Go objects.
// The caller must not modify Response bytes during Evaluate.
type Input struct {
	// Context contains validated params and copied configuration, metadata, and
	// identities. Each evaluation receives a fresh native JavaScript copy.
	Context *render.Context

	// Response is an object of caller-prepared response fields. Nil means {}.
	// The HTTP executor will prepare status, headers, body, and mediaType; this
	// primitive does not read streams or interpret HTTP content types.
	Response common.RawJSON
}

// Expression is an immutable, reusable pure expression program. Evaluation
// always creates a fresh VM, so callers can reuse it concurrently. It has no
// asynchronous host, connector helper library, or network capability.
type Expression struct {
	// program owns compilation, fresh-VM execution, and JSON output conversion.
	program *apjsruntime.Expression

	// limits is the same captured policy used by the Tool input adapter.
	limits Limits
}

// CompileExpression prepares a shared pure expression with Tool input bindings.
// Compilation never executes authored code and returns safe classifications for
// invalid source or policy. The effective limits are copied for input binding.
func CompileExpression(source string, limits Limits) (*Expression, error) {
	program, err := apjsruntime.CompileExpression(source, limits)
	if err != nil {
		return nil, err
	}
	return &Expression{program: program, limits: limits}, nil
}

// Evaluate returns a detached JSON value or an error with no partial output.
// Native objects can be mutated during this call without changing the captured
// snapshot or another invocation. The result uses JavaScript Number semantics;
// this is not a precision-preserving raw JSON passthrough.
//
// Cancellation and the configured timeout remain active through conversion.
// A configured timeout is apjsruntime.ErrLimitExceeded; caller cancellation and
// deadlines keep ctx.Err(), taking precedence if both expire before return.
// Only the interrupt watcher accesses the VM from another goroutine, through
// Goja's thread-safe Interrupt method. Thrown values and stacks are never exposed.
func (e *Expression) Evaluate(
	ctx context.Context,
	input Input,
) (common.RawJSON, error) {
	if e == nil || e.program == nil || ctx == nil {
		return nil, apjsruntime.ErrInvalidInput
	}

	result, err := e.program.Evaluate(ctx, func(runCtx context.Context, vm *goja.Runtime) error {
		return bindInput(runCtx, vm, input, e.limits)
	})
	return common.RawJSON(result), err
}
