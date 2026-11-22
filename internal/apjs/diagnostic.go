package apjs

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// FailureDiagnostic contains host-controlled details that trusted adapters may
// log for a failed connector evaluation. It never contains source text, thrown
// values, property names, or a retained JavaScript error or stack.
type FailureDiagnostic struct {
	// Category is the fixed safe error classification, including cancellation.
	Category string
	// Phase identifies the host's current evaluation stage, such as "execute"
	// or "output"; it is never taken from authored code.
	Phase string
	// OutputReason identifies a fixed unsupported-output condition, or is empty
	// when no such rejection survives the final error selection. It contains no
	// property names, paths, or values; limits and cancellation have no reason.
	OutputReason jsruntime.OutputRejectionReason
	// Source identifies "library" or "expression" when a known source unit
	// supplied the position. It is empty when no position can be attributed.
	Source string
	// Line and Column are one-based coordinates, or zero when unavailable.
	Line   int
	Column int
}

// failureError keeps diagnostic metadata separate from the public error text.
// Its only cause is a safe category; the original Goja error cannot escape or
// retain VM state through either Error or Unwrap.
type failureError struct {
	category   error
	diagnostic FailureDiagnostic
}

// Error preserves the opaque classification expected by API callers.
func (e *failureError) Error() string { return e.category.Error() }

// Unwrap preserves errors.Is checks without exposing the authored cause.
func (e *failureError) Unwrap() error { return e.category }

// Diagnostic returns detached, safe details for trusted logging. Ordinary
// errors have no diagnostic; outer contextual error wrappers are supported.
func Diagnostic(err error) (FailureDiagnostic, bool) {
	var failure *failureError
	if !errors.As(err, &failure) {
		return FailureDiagnostic{}, false
	}
	return failure.diagnostic, true
}

// withFailureDiagnostic attaches safe metadata after the runtime has selected
// the final error, so cancellation and execution limits keep their precedence.
// Unknown host errors fall back to evaluation failure rather than retaining an
// error whose message or cause may include private data.
func withFailureDiagnostic(err error, diagnostic FailureDiagnostic) error {
	if err == nil {
		return nil
	}
	category := jsruntime.ErrEvaluation
	for _, candidate := range []error{
		context.Canceled, context.DeadlineExceeded, jsruntime.ErrInvalidSource,
		jsruntime.ErrInvalidInput, jsruntime.ErrInvalidOutput,
		jsruntime.ErrLimitExceeded, jsruntime.ErrEvaluation,
	} {
		if errors.Is(err, candidate) {
			category = candidate
			break
		}
	}
	diagnostic.Category = category.Error()
	// Always recompute from the final error. A saved diagnostic may describe an
	// earlier conversion that cancellation or a shared-budget failure superseded.
	diagnostic.OutputReason = ""
	if category == jsruntime.ErrInvalidOutput {
		if reason, ok := jsruntime.OutputReason(err); ok {
			diagnostic.OutputReason = reason
		} else if existing, ok := Diagnostic(err); ok {
			// Detached connector transforms wrap an already-sanitized evaluation
			// error a second time without retaining its original runtime cause.
			diagnostic.OutputReason = existing.OutputReason
		}
	}
	return &failureError{category: category, diagnostic: diagnostic}
}

// LibraryCompilationError converts a compiler failure into the opaque source
// category while preserving only its numeric location for trusted diagnostics.
// The compiler's message, filename, source, and underlying error are discarded.
// Nil remains nil.
func LibraryCompilationError(err error) error {
	if err == nil {
		return nil
	}
	diagnostic := FailureDiagnostic{Phase: "library-compile"}
	diagnostic.setCompilePosition(err, "library")
	return withFailureDiagnostic(jsruntime.ErrInvalidSource, diagnostic)
}

// setCompilePosition copies structured parser/compiler coordinates without
// formatting an error or retaining any source-bearing objects.
func (d *FailureDiagnostic) setCompilePosition(err error, source string) {
	var parseErrors parser.ErrorList
	var compileError *goja.CompilerSyntaxError
	if errors.As(err, &parseErrors) && len(parseErrors) > 0 && parseErrors[0] != nil {
		d.setPosition(source, parseErrors[0].Position.Line, parseErrors[0].Position.Column)
	} else if errors.As(err, &compileError) && compileError.File != nil && compileError.Offset >= 0 {
		position := compileError.File.Position(compileError.Offset)
		d.setPosition(source, position.Line, position.Column)
	}
}

// setExceptionPosition reads captured VM metadata without inspecting or
// coercing the thrown value. Dynamic eval/Function frames are skipped: only
// known host-assigned source units can contribute a location, never their
// authored function names or arbitrary filenames.
func (d *FailureDiagnostic) setExceptionPosition(err error) {
	var exception *goja.Exception
	if !errors.As(err, &exception) {
		return
	}
	for _, frame := range exception.Stack() {
		var source string
		switch frame.SrcName() {
		case "connector.js":
			source = "library"
		case "expression.js":
			source = "expression"
		default:
			continue
		}
		position := frame.Position()
		d.setPosition(source, position.Line, position.Column)
		if d.Line > 0 {
			return
		}
	}
}

// setPosition accepts only useful one-based coordinates and fixed source units.
func (d *FailureDiagnostic) setPosition(source string, line, column int) {
	if (source == "library" || source == "expression") && line > 0 && column > 0 {
		d.Source, d.Line, d.Column = source, line, column
	}
}
