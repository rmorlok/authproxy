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
	outputOptions jsruntime.JSONOutputOptions,
	convert func(context.Context, *goja.Runtime, goja.Value) (goja.Value, error),
) (result json.RawMessage, err error) {
	diagnostic := FailureDiagnostic{Phase: "source"}
	var locatedError error
	defer func() {
		if err != nil {
			// The runtime may replace an authored failure with cancellation or a
			// policy timeout. Do not attribute the final failure to a stale throw.
			if locatedError != nil && err != locatedError {
				diagnostic.Source, diagnostic.Line, diagnostic.Column = "", 0, 0
			}
			err = withFailureDiagnostic(err, diagnostic)
		}
	}()

	// Capture metadata before the classifier discards the Goja error. Neither
	// metadata nor the returned safe category retains the original exception.
	evaluationError := func(cause error) error {
		diagnostic.setExceptionPosition(cause)
		locatedError = jsruntime.EvaluationError(cause)
		return locatedError
	}

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
	if err := CheckCombinedSource(librarySource, expression, limits.MaxSourceBytes); err != nil {
		return nil, err
	}

	if !utf8.ValidString(expression) ||
		!utf8.ValidString(librarySource) ||
		strings.TrimSpace(expression) == "" {
		return nil, jsruntime.ErrInvalidSource
	}

	return jsruntime.EvaluateJSONWithOptions(
		ctx,
		limits,
		outputOptions,
		func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
			diagnostic = FailureDiagnostic{Phase: "expression-compile"}
			// Preserve legacy script syntax and non-strict semantics. Compilation is
			// byte-bounded; Go parsing is cooperative only at the surrounding checks.
			program, err := compileSource("expression.js", expression)
			if err != nil {
				diagnostic.setCompilePosition(err, "expression")
				locatedError = jsruntime.ErrInvalidSource
				return nil, jsruntime.ErrInvalidSource
			}

			diagnostic = FailureDiagnostic{Phase: "input"}
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

			diagnostic = FailureDiagnostic{Phase: "library-initialize"}
			if c.library != nil && c.library.program != nil {
				// Library.Run formats its legacy diagnostic, which can call an
				// authored thrown value's toString. The bounded path never needs it.
				if _, err := vm.RunProgram(c.library.program); err != nil {
					return nil, evaluationError(err)
				}
			}

			diagnostic = FailureDiagnostic{Phase: "reserved-bindings"}
			// Reading a reserved global can invoke a library-installed accessor.
			// Catch authored exceptions without hiding uncatchable runtime limits.
			if exception := vm.Try(
				func() { err = validateReservedRuntimeVars(vm) },
			); exception != nil {
				return nil, evaluationError(exception)
			}
			if err != nil {
				return nil, jsruntime.ErrInvalidSource
			}

			diagnostic = FailureDiagnostic{Phase: "input-bindings"}
			variables := values[0].(*goja.Object)
			for _, name := range variables.Keys() {
				if err := runCtx.Err(); err != nil {
					return nil, err
				}
				if err := vm.Set(name, variables.Get(name)); err != nil {
					return nil, evaluationError(err)
				}
			}

			if err := runCtx.Err(); err != nil {
				return nil, err
			}

			diagnostic = FailureDiagnostic{Phase: "execute"}
			value, err := vm.RunProgram(program)
			if err != nil {
				return nil, evaluationError(err)
			}

			diagnostic = FailureDiagnostic{Phase: "output"}
			return convert(runCtx, vm, value)
		},
	)
}
