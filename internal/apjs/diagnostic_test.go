package apjs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestFailureDiagnosticPositions preserves useful locations without retaining
// authored errors, source text, function names, or configuration-derived data.
func TestFailureDiagnosticPositions(t *testing.T) {
	for name, test := range map[string]struct {
		library, source, phase, unit string
		line                         int
		category                     error
	}{
		"expression syntax":  {source: "\nconst privateDetail = ;", phase: "expression-compile", unit: "expression", line: 2, category: jsruntime.ErrInvalidSource},
		"expression compile": {source: "let privateDetail = 1;\nlet privateDetail = 2;", phase: "expression-compile", unit: "expression", line: 2, category: jsruntime.ErrInvalidSource},
		"expression throw":   {source: "\n\nthrow cfg.secret;", phase: "execute", unit: "expression", line: 3, category: jsruntime.ErrEvaluation},
		"library throw":      {library: "\nthrow 'privateDetail';", source: "({})", phase: "library-initialize", unit: "library", line: 2, category: jsruntime.ErrEvaluation},
		"library helper":     {library: "function privateFunction() {\nthrow cfg.secret;\n}", source: "privateFunction()", phase: "execute", unit: "library", line: 2, category: jsruntime.ErrEvaluation},
	} {
		t.Run(name, func(t *testing.T) {
			// Connector inputs and exceptions deliberately carry private values.
			library, err := CompileLibrary(test.library)
			require.NoError(t, err)
			_, err = library.NewContext(map[string]any{"cfg": map[string]any{"secret": "privateDetail"}}).
				EvaluateObjectJSONWithContext(context.Background(), test.source, boundedObjectTestLimits())

			// API errors stay opaque while trusted logging receives only fixed labels and coordinates.
			require.ErrorIs(t, err, test.category)
			require.Equal(t, test.category.Error(), err.Error())
			diagnostic, ok := Diagnostic(fmt.Errorf("trusted wrapper: %w", err))
			require.True(t, ok)
			require.Equal(t, test.category.Error(), diagnostic.Category)
			require.Equal(t, test.phase, diagnostic.Phase)
			require.Equal(t, test.unit, diagnostic.Source)
			require.Equal(t, test.line, diagnostic.Line)
			require.Positive(t, diagnostic.Column)
			require.NotContains(t, fmt.Sprintf("%+v", diagnostic), "private")

			// Even error-chain inspection cannot recover a Goja value or parser source.
			var exception *goja.Exception
			var parseErrors parser.ErrorList
			require.False(t, errors.As(err, &exception))
			require.False(t, errors.As(err, &parseErrors))
			require.Same(t, test.category, errors.Unwrap(err))
		})
	}
}

// TestLibraryCompilationDiagnostic keeps cached library compilation failures
// opaque while making the same safe position available as execution failures.
func TestLibraryCompilationDiagnostic(t *testing.T) {
	// The compiler retains private source in its error; the adapter must discard it.
	_, compileErr := CompileLibrary("\nconst privateDetail = ;")
	require.Error(t, compileErr)
	err := LibraryCompilationError(compileErr)
	require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
	require.Equal(t, jsruntime.ErrInvalidSource.Error(), err.Error())
	diagnostic, ok := Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, "library-compile", diagnostic.Phase)
	require.Equal(t, "library", diagnostic.Source)
	require.Equal(t, 2, diagnostic.Line)
	require.Positive(t, diagnostic.Column)
	require.Same(t, jsruntime.ErrInvalidSource, errors.Unwrap(err))

	// AST compilation has a different error shape from parsing, with equally safe coordinates.
	_, compileErr = CompileLibrary("let privateDetail = 1;\nlet privateDetail = 2;")
	require.Error(t, compileErr)
	err = LibraryCompilationError(compileErr)
	diagnostic, ok = Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, 2, diagnostic.Line)
	require.Equal(t, 5, diagnostic.Column)
	var compilerError *goja.CompilerSyntaxError
	require.False(t, errors.As(err, &compilerError))

	// An unexpected compiler error has no location and cannot smuggle its message out.
	err = LibraryCompilationError(errors.New("privateDetail"))
	diagnostic, ok = Diagnostic(err)
	require.True(t, ok)
	require.Empty(t, diagnostic.Source)
	require.Zero(t, diagnostic.Line)
	require.NotContains(t, fmt.Sprintf("%+v %v", diagnostic, err), "privateDetail")
	require.Nil(t, LibraryCompilationError(nil))
	_, ok = Diagnostic(errors.New("ordinary error"))
	require.False(t, ok)
}

// TestFailureDiagnosticDoesNotCoerceThrownValues ensures metadata extraction
// never calls an exception's authored string conversion, even during library init.
func TestFailureDiagnosticDoesNotCoerceThrownValues(t *testing.T) {
	const source = `throw {toString() { for (;;) {} }};`
	for _, initialize := range []bool{false, true} {
		t.Run(fmt.Sprintf("library=%t", initialize), func(t *testing.T) {
			librarySource, expression := "", source
			if initialize {
				librarySource, expression = source, "({})"
			}
			library, err := CompileLibrary(librarySource)
			require.NoError(t, err)
			limits := boundedObjectTestLimits()
			limits.Timeout = 100 * time.Millisecond

			// Calling toString would loop until the policy timeout, changing the category.
			_, err = library.NewContext(nil).EvaluateObjectJSONWithContext(context.Background(), expression, limits)
			require.ErrorIs(t, err, jsruntime.ErrEvaluation)
			diagnostic, ok := Diagnostic(err)
			require.True(t, ok)
			require.Positive(t, diagnostic.Line)
		})
	}
}

// TestFailureDiagnosticOutputOmitsPropertyNames identifies the conversion stage
// without logging secret-derived property names or invoking rejected accessors.
func TestFailureDiagnosticOutputOmitsPropertyNames(t *testing.T) {
	for source, reason := range map[string]jsruntime.OutputRejectionReason{
		`new Date(0)`:                                             jsruntime.OutputUnexpectedType,
		`({[cfg.secret]: function() {}})`:                         jsruntime.OutputUnsupportedObject,
		`({get [cfg.secret]() { for (;;) {} }})`:                  jsruntime.OutputAccessor,
		`const result = {}; result[cfg.secret] = result; result;`: jsruntime.OutputCycle,
	} {
		// Neither a root rejection nor an encoder rejection can expose a JSON path.
		_, err := NewContext(nil, map[string]any{"cfg": map[string]any{"secret": "privateDetail"}}).
			EvaluateObjectJSONWithContext(context.Background(), source, boundedObjectTestLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
		diagnostic, ok := Diagnostic(err)
		require.True(t, ok)
		require.Equal(t, FailureDiagnostic{
			Category: jsruntime.ErrInvalidOutput.Error(), Phase: "output", OutputReason: reason,
		}, diagnostic)
		require.NotContains(t, fmt.Sprintf("%+v %v", diagnostic, err), "privateDetail")
	}
}

// TestFailureDiagnosticOutputReasonsAcrossAdapters covers root-contract failures
// and nested shared-encoder errors retained through detached option conversion.
func TestFailureDiagnosticOutputReasonsAcrossAdapters(t *testing.T) {
	for _, test := range []struct {
		name, kind, source string
		reason             jsruntime.OutputRejectionReason
	}{
		{"object root", "object", `"privateDetail"`, jsruntime.OutputUnexpectedType},
		{"predicate root", "boolean", `({[cfg.secret]: true})`, jsruntime.OutputUnexpectedType},
		{"options root", "options", `({[cfg.secret]: true})`, jsruntime.OutputInvalidShape},
		{"option item", "options", `["privateDetail"]`, jsruntime.OutputInvalidShape},
		{"missing option field", "options", `[{value: "privateDetail"}]`, jsruntime.OutputInvalidShape},
		{"nested option accessor", "options", `[{value: {get [cfg.secret]() {for (;;) {}}}, label: "label"}]`, jsruntime.OutputAccessor},
		{"nested option cycle", "options", `const value = {}; value[cfg.secret] = value; [{value, label: "label"}]`, jsruntime.OutputCycle},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Each adapter sees a private key/value, but diagnoses only its fixed contract failure.
			jsctx := NewContext(nil, map[string]any{"cfg": map[string]any{"secret": "privateDetail"}})
			var err error
			switch test.kind {
			case "object":
				_, err = jsctx.EvaluateObjectJSONWithContext(context.Background(), test.source, boundedObjectTestLimits())
			case "boolean":
				_, err = jsctx.EvaluateBooleanWithContext(context.Background(), test.source, boundedObjectTestLimits())
			case "options":
				_, err = jsctx.TransformOptionsWithContext(context.Background(), test.source, boundedObjectTestLimits())
			}

			// TransformOptions wraps the evaluation error again after native JSON conversion.
			require.ErrorIs(t, err, jsruntime.ErrInvalidOutput)
			require.EqualError(t, err, jsruntime.ErrInvalidOutput.Error())
			diagnostic, ok := Diagnostic(fmt.Errorf("trusted wrapper: %w", err))
			require.True(t, ok)
			require.Equal(t, test.reason, diagnostic.OutputReason)
			require.Equal(t, "output", diagnostic.Phase)
			require.NotContains(t, fmt.Sprintf("%+v %v", diagnostic, err), "privateDetail")
			require.Same(t, jsruntime.ErrInvalidOutput, errors.Unwrap(err))
		})
	}
}

// TestFailureDiagnosticOutputReasonRewrapping carries only a still-relevant
// reason through connector wrappers and clears stale metadata on replacement.
func TestFailureDiagnosticOutputReasonRewrapping(t *testing.T) {
	original := withFailureDiagnostic(jsruntime.RejectOutput(jsruntime.OutputCycle), FailureDiagnostic{Phase: "output"})
	previous, ok := Diagnostic(original)
	require.True(t, ok)

	// A second trusted wrapper preserves the already-sanitized conversion reason.
	err := withFailureDiagnostic(fmt.Errorf("trusted wrapper: %w", original), FailureDiagnostic{Phase: "output"})
	diagnostic, ok := Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, previous, diagnostic)

	for _, final := range []error{
		context.Canceled, context.DeadlineExceeded, jsruntime.ErrLimitExceeded,
		jsruntime.ErrEvaluation, jsruntime.ErrInvalidOutput, errors.New("privateDetail"),
	} {
		// A saved output reason must not survive replacement by a different failure.
		err := withFailureDiagnostic(final, previous)
		diagnostic, ok := Diagnostic(err)
		require.True(t, ok)
		require.Empty(t, diagnostic.OutputReason)
		require.NotContains(t, fmt.Sprintf("%+v %v", diagnostic, err), "privateDetail")
	}
}

// TestFailureDiagnosticCancellation keeps the runtime's final cancellation and
// timeout decisions instead of attributing them to a preliminary output failure.
func TestFailureDiagnosticCancellation(t *testing.T) {
	// Cancellation during trusted conversion overrides the converter's rejection.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := NewContext(nil, nil).evaluateJSONWithContext(ctx, "({})", boundedObjectTestLimits(), jsruntime.JSONOutputOptions{},
		func(context.Context, *goja.Runtime, goja.Value) (goja.Value, error) {
			cancel()
			return nil, jsruntime.RejectOutput(jsruntime.OutputCycle)
		})
	require.ErrorIs(t, err, context.Canceled)
	diagnostic, ok := Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, FailureDiagnostic{Category: context.Canceled.Error(), Phase: "output"}, diagnostic)

	// A VM timeout has a stage but no misleading exception location.
	limits := boundedObjectTestLimits()
	limits.Timeout = 20 * time.Millisecond
	_, err = NewContext(nil, nil).EvaluateObjectJSONWithContext(context.Background(), "for (;;) {}", limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	diagnostic, ok = Diagnostic(err)
	require.True(t, ok)
	require.Equal(t, FailureDiagnostic{Category: jsruntime.ErrLimitExceeded.Error(), Phase: "execute"}, diagnostic)

	// A timeout after a trusted converter observes rejection likewise discards its reason.
	_, err = NewContext(nil, nil).evaluateJSONWithContext(context.Background(), "({})", limits, jsruntime.JSONOutputOptions{},
		func(runCtx context.Context, _ *goja.Runtime, _ goja.Value) (goja.Value, error) {
			<-runCtx.Done()
			return nil, jsruntime.RejectOutput(jsruntime.OutputCycle)
		})
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	diagnostic, ok = Diagnostic(err)
	require.True(t, ok)
	require.Empty(t, diagnostic.OutputReason)
}

// TestFailureDiagnosticConcurrentReuse keeps each evaluation's stage and
// location private even when a compiled library and input snapshot are shared.
func TestFailureDiagnosticConcurrentReuse(t *testing.T) {
	library, err := CompileLibrary(`function ready() { return true; }`)
	require.NoError(t, err)
	jsctx := library.NewContext(map[string]any{"cfg": map[string]any{"secret": "privateDetail"}})
	type outcome struct {
		line int
		err  error
	}
	results := make(chan outcome, 12)
	var work sync.WaitGroup

	// Each concurrently evaluated statement list throws at a distinct source line.
	for line := 1; line <= cap(results); line++ {
		work.Go(func() {
			_, err := jsctx.EvaluateObjectJSONWithContext(context.Background(), strings.Repeat("\n", line-1)+"throw cfg.secret;", boundedObjectTestLimits())
			results <- outcome{line: line, err: err}
		})
	}
	work.Wait()
	close(results)
	for result := range results {
		require.ErrorIs(t, result.err, jsruntime.ErrEvaluation)
		diagnostic, ok := Diagnostic(result.err)
		require.True(t, ok)
		require.Equal(t, "execute", diagnostic.Phase)
		require.Equal(t, "expression", diagnostic.Source)
		require.Equal(t, result.line, diagnostic.Line)
	}
}
