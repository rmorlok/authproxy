package apjs

import (
	"context"
	"reflect"

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
	result, err := c.evaluateJSONWithContext(
		ctx, expression, limits,
		func(_ context.Context, vm *goja.Runtime, value goja.Value) (goja.Value, error) {
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
