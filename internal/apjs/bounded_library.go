package apjs

import (
	"context"
	"unicode/utf8"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// CompileAndValidateLibraryWithContext compiles a reusable connector library and
// checks its initialization in a fresh bounded VM without runtime variables or
// host capabilities. Source is bounded before parsing; the supplied deadline and
// stack limit cover initialization and reserved-name checks. Parsing itself is
// not interruptible, but cancellation is checked before and after it.
//
// All limits must be positive, even though validation returns no VM data. Errors
// expose only safe runtime categories or caller cancellation, and always return
// a nil library. Successful libraries retain their original source for combined
// library/expression accounting and may be reused concurrently in fresh VMs.
func CompileAndValidateLibraryWithContext(
	ctx context.Context,
	source string,
	limits jsruntime.Limits,
) (*Library, error) {
	if ctx == nil {
		return nil, jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if len(source) > limits.MaxSourceBytes {
		return nil, jsruntime.ErrLimitExceeded
	}
	if !utf8.ValidString(source) {
		return nil, jsruntime.ErrInvalidSource
	}

	var library *Library
	err := jsruntime.Run(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) error {
		var err error
		library, err = CompileLibrary(source)
		if contextErr := runCtx.Err(); contextErr != nil {
			return contextErr
		}
		if err != nil {
			return jsruntime.ErrInvalidSource
		}
		// The legacy compiler drops whitespace-only source. Retain those bytes
		// so later predicate checks cannot undercount the combined source size.
		library.source = source
		if err := library.Run(vm); err != nil {
			return jsruntime.EvaluationError(err)
		}
		// A reserved global may be an authored accessor. Catch its exception
		// while leaving interrupts and stack limits to the shared lifecycle.
		if exception := vm.Try(func() { err = validateReservedRuntimeVars(vm) }); exception != nil {
			return jsruntime.EvaluationError(exception)
		}
		if err != nil {
			return jsruntime.ErrInvalidSource
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return library, nil
}
