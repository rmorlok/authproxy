package apjs

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// evaluateJSONWithContext runs a connector statement list with copied native
// inputs and converts its result under one shared deadline. The trusted convert
// callback normalizes the domain's root value before bounded JSON encoding; it
// must honor runCtx during Go work and return only safe error classifications.
// No VM value escapes. Callers must bound raw library source before compilation.
func (c Context) evaluateJSONWithContext(
	ctx context.Context,
	expression string,
	limits jsruntime.Limits,
	convert func(context.Context, *goja.Runtime, goja.Value) (goja.Value, error),
) (json.RawMessage, error) {
	if ctx == nil {
		return nil, jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}

	librarySource := c.library.Source()
	if len(expression) > limits.MaxSourceBytes ||
		len(librarySource) > limits.MaxSourceBytes-len(expression) {
		return nil, jsruntime.ErrLimitExceeded
	}

	if !utf8.ValidString(expression) ||
		!utf8.ValidString(librarySource) ||
		strings.TrimSpace(expression) == "" {
		return nil, jsruntime.ErrInvalidSource
	}

	return jsruntime.EvaluateJSON(
		ctx,
		limits,
		func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
			// Preserve legacy script syntax and non-strict semantics. Compilation is
			// byte-bounded; Go parsing is cooperative only at the surrounding checks.
			program, err := compileSource("expression.js", expression)
			if err != nil {
				return nil, jsruntime.ErrInvalidSource
			}

			inputLimits := jsruntime.JSONLimits{
				MaxBytes: limits.MaxInputBytes,
				MaxDepth: limits.MaxDepth,
				MaxNodes: limits.MaxNodes,
			}
			encoded, err := encodeVariables(runCtx, c.vars, inputLimits)
			if err != nil {
				return nil, err
			}

			// Decode before authored initialization can replace JSON.parse or other
			// intrinsics, but keep the copied values private until the library runs.
			decoder, err := jsruntime.NewJSONDecoder(vm)
			if err != nil {
				return nil, err
			}

			values, err := decoder.Decode(runCtx, inputLimits, encoded)
			if err != nil {
				return nil, err
			}

			if err := c.library.Run(vm); err != nil {
				return nil, jsruntime.EvaluationError(err)
			}

			// Reading a reserved global can invoke a library-installed accessor.
			// Catch authored exceptions without hiding uncatchable runtime limits.
			if exception := vm.Try(
				func() { err = validateReservedRuntimeVars(vm) },
			); exception != nil {
				return nil, jsruntime.EvaluationError(exception)
			}
			if err != nil {
				return nil, jsruntime.ErrInvalidSource
			}

			variables := values[0].(*goja.Object)
			for _, name := range variables.Keys() {
				if err := runCtx.Err(); err != nil {
					return nil, err
				}
				if err := vm.Set(name, variables.Get(name)); err != nil {
					return nil, jsruntime.EvaluationError(err)
				}
			}

			if err := runCtx.Err(); err != nil {
				return nil, err
			}

			value, err := vm.RunProgram(program)
			if err != nil {
				return nil, jsruntime.EvaluationError(err)
			}

			return convert(runCtx, vm, value)
		},
	)
}
