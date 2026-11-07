// Package javascript evaluates trusted async Tool scripts and pure expressions
// with native JavaScript copies of captured data. I/O is available only when a
// Tool caller explicitly supplies a captured connection's HTTP session.
package javascript

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
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
	program *goja.Program
	limits  Limits
}

// CompileExpression parses exactly one strict JavaScript expression without
// executing authored code. Function-call expressions and IIFEs are allowed;
// statements cannot escape the expression wrapper. Syntax failures are safe
// classifications rather than parser messages containing source text.
func CompileExpression(source string, limits Limits) (*Expression, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if len(source) > limits.MaxSourceBytes {
		return nil, apjsruntime.ErrLimitExceeded
	}
	if !utf8.ValidString(source) || strings.TrimSpace(source) == "" {
		return nil, apjsruntime.ErrInvalidSource
	}

	parsed, err := parser.ParseFile(
		nil, // fileSet
		"tool-expression.js",
		"(\n"+source+"\n)",
		0, // mode
		parser.WithDisableSourceMaps,
	)
	if err != nil || len(parsed.Body) != 1 {
		return nil, apjsruntime.ErrInvalidSource
	}

	if _, ok := parsed.Body[0].(*ast.ExpressionStatement); !ok {
		return nil, apjsruntime.ErrInvalidSource
	}

	program, err := goja.CompileAST(parsed, true)
	if err != nil {
		return nil, apjsruntime.ErrInvalidSource
	}

	return &Expression{program: program, limits: limits}, nil
}

// Evaluate returns a detached JSON value or an error with no partial output.
// Native objects can be mutated during this call without changing the captured
// snapshot or another invocation. The result uses JavaScript Number semantics;
// this is not a precision-preserving raw JSON passthrough.
//
// Cancellation and the configured timeout remain active through conversion.
// A configured timeout is apjsruntime.ErrLimitExceeded; caller cancellation/deadlines keep
// ctx.Err(), taking precedence if both have expired before evaluation returns.
// Only the interrupt watcher accesses the VM from another goroutine, through
// Goja's thread-safe Interrupt method. Thrown values and stacks are never exposed.
func (e *Expression) Evaluate(
	ctx context.Context,
	input Input,
) (common.RawJSON, error) {
	if e == nil || e.program == nil || ctx == nil {
		return nil, apjsruntime.ErrInvalidInput
	}

	return evaluateJSON(ctx, e.limits, input, func(_ context.Context, vm *goja.Runtime) (goja.Value, error) {
		value, err := vm.RunProgram(e.program)
		if err != nil {
			return nil, runtimeEvaluationError(err)
		}
		return value, nil
	})
}
