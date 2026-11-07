package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// Expression is an immutable, reusable pure expression program. Each Evaluate
// call owns a fresh VM and detached JSON output, so a compiled expression may
// serve concurrent callers. It installs no domain globals or I/O capabilities.
type Expression struct {
	// program is strict compiled code shared across otherwise isolated VMs.
	program *goja.Program

	// limits captures the effective positive policy at compilation time.
	limits Limits
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
		return nil, ErrLimitExceeded
	}
	if !utf8.ValidString(source) || strings.TrimSpace(source) == "" {
		return nil, ErrInvalidSource
	}

	parsed, err := parser.ParseFile(
		nil, // fileSet
		"expression.js",
		"(\n"+source+"\n)",
		0, // mode
		parser.WithDisableSourceMaps,
	)
	if err != nil || len(parsed.Body) != 1 {
		return nil, ErrInvalidSource
	}

	if _, ok := parsed.Body[0].(*ast.ExpressionStatement); !ok {
		return nil, ErrInvalidSource
	}

	program, err := goja.CompileAST(parsed, true)
	if err != nil {
		return nil, ErrInvalidSource
	}

	return &Expression{program: program, limits: limits}, nil
}

// Evaluate binds optional input, executes the expression, and converts its
// result to detached JSON under one deadline. A nil prepare callback leaves the
// fresh VM's standard globals unchanged. Unsupported output, including Promises,
// fails with ErrInvalidOutput; cancellation or failure returns no partial JSON.
//
// prepare is trusted adapter code, called before the expression. It may bind
// copied native JSON and selected pure helpers, but must not install I/O or live
// connection capabilities. It must honor runCtx, enforce the captured input
// limits (for example with JSONDecoder), and return only safe errors. VM access
// stays on the calling goroutine and neither the VM nor its values may escape.
// The output encoder captures pristine intrinsics before prepare runs.
//
// A policy timeout is ErrLimitExceeded. Caller cancellation/deadlines retain
// their context error and take precedence when both contexts have expired.
func (e *Expression) Evaluate(
	ctx context.Context,
	prepare func(context.Context, *goja.Runtime) error,
) (json.RawMessage, error) {
	if e == nil || e.program == nil || ctx == nil {
		return nil, ErrInvalidInput
	}
	return EvaluateJSON(ctx, e.limits, func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
		if prepare != nil {
			if err := prepare(runCtx, vm); err != nil {
				return nil, err
			}
		}
		// Do not begin authored execution after preparation used the deadline.
		if err := runCtx.Err(); err != nil {
			return nil, err
		}
		value, err := vm.RunProgram(e.program)
		return value, EvaluationError(err)
	})
}
