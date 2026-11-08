package apjs

import (
	"context"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// EvaluateBooleanWithContext evaluates a connector predicate in a fresh bounded
// VM, with caller cancellation covering input copying, library initialization,
// execution, and result conversion. The caller supplies positive effective
// limits; expression and library source share MaxSourceBytes. Library compilation
// happens before Context construction, so callers must also bound source before
// compiling a library. No HTTP or other host capabilities are installed.
//
// Like EvaluateBoolean, this accepts a non-strict JavaScript statement list whose
// final value must be a boolean (including JavaScript Boolean wrappers, as in
// the legacy method). The library initializes before input globals are bound
// and cannot claim cfg, labels, annotations, or data. Unlike the
// legacy method, inputs must be supported JSON data (see encodeVariables); they
// are copied into native values, so authored mutations never change caller data.
// The caller must keep its variable maps/slices stable until this call returns.
//
// Failures return false and safe shared-runtime errors without source, private
// input, thrown values, or parser diagnostics. Caller cancellation retains its
// context error; policy expiry returns ErrLimitExceeded. The existing unbounded
// method remains available while other connector callers migrate deliberately.
func (c Context) EvaluateBooleanWithContext(
	ctx context.Context,
	expression string,
	limits jsruntime.Limits,
) (bool, error) {
	if ctx == nil {
		return false, jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := limits.Validate(); err != nil {
		return false, err
	}

	librarySource := c.library.Source()
	if len(expression) > limits.MaxSourceBytes ||
		len(librarySource) > limits.MaxSourceBytes-len(expression) {
		return false, jsruntime.ErrLimitExceeded
	}

	if !utf8.ValidString(expression) ||
		!utf8.ValidString(librarySource) ||
		strings.TrimSpace(expression) == "" {
		return false, jsruntime.ErrInvalidSource
	}

	result, err := jsruntime.EvaluateJSON(
		ctx,
		limits,
		func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
			// Preserve legacy script syntax and non-strict semantics. Compilation is
			// byte-bounded; Go parsing is cooperative only at the surrounding checks.
			program, err := compileSource("predicate.js", expression)
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

			// Inspect the export type before exporting anything: arbitrary objects
			// could invoke getters or traverse an unbounded graph. Goja's boolean
			// primitive and Boolean wrapper both export one native bool without
			// coercion or property access, preserving the legacy conversion contract.
			if value == nil || value.ExportType() != reflect.TypeFor[bool]() {
				return nil, jsruntime.ErrInvalidOutput
			}

			return vm.ToValue(value.Export().(bool)), nil
		},
	)
	if err != nil {
		return false, err
	}

	return string(result) == "true", nil
}
