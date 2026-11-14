package apjs

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// CompileAndValidateLibraryWithContext compiles a reusable connector library and
// checks its initialization in a fresh bounded VM without runtime variables or
// host capabilities. Source is bounded before parsing; the supplied deadline and
// stack limit cover initialization and reserved-name checks. Parsing itself is
// not interruptible, but cancellation is checked before and after it.
//
// All limits must be positive, even though validation returns no VM data. Errors
// expose safe runtime categories or caller cancellation, with a syntax position
// or conflicting reserved global where available. Source, parser messages, and
// thrown values are omitted. Failures always return a nil library. Successful
// libraries retain their original source for combined library/expression
// accounting and may be reused concurrently in fresh VMs.
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
			return libraryCompileValidationError(err)
		}
		// The legacy compiler drops whitespace-only source. Retain those bytes
		// so later predicate checks cannot undercount the combined source size.
		library.source = source
		if err := library.Run(vm); err != nil {
			return jsruntime.EvaluationError(err)
		}
		// A reserved global may be an authored accessor. Catch its exception
		// while leaving interrupts and stack limits to the shared lifecycle.
		var reservedName string
		if exception := vm.Try(func() { reservedName = findReservedRuntimeVar(vm) }); exception != nil {
			return jsruntime.EvaluationError(exception)
		}
		if reservedName != "" {
			return fmt.Errorf("%w: reserved runtime variable %q", jsruntime.ErrInvalidSource, reservedName)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return library, nil
}

// libraryCompileValidationError retains only a one-based syntax position for
// definition authors. Neither parser/compiler messages nor their underlying
// errors escape: they can retain source text and filenames. Unknown positions
// fall back to the shared category; errors.Is still recognizes ErrInvalidSource.
func libraryCompileValidationError(err error) error {
	var line, column int
	var parseErrors parser.ErrorList
	var compileError *goja.CompilerSyntaxError
	if errors.As(err, &parseErrors) && len(parseErrors) > 0 && parseErrors[0] != nil {
		line, column = parseErrors[0].Position.Line, parseErrors[0].Position.Column
	} else if errors.As(err, &compileError) && compileError.File != nil && compileError.Offset >= 0 {
		position := compileError.File.Position(compileError.Offset)
		line, column = position.Line, position.Column
	}
	if line > 0 && column > 0 {
		return fmt.Errorf("%w at line %d, column %d", jsruntime.ErrInvalidSource, line, column)
	}
	return jsruntime.ErrInvalidSource
}
