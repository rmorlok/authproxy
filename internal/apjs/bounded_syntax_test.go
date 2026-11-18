package apjs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// TestCombinedSourceAdmission keeps raw-byte admission identical for callers
// checking definitions before compilation and callers reusing a compiled library.
func TestCombinedSourceAdmission(t *testing.T) {
	for _, test := range []struct {
		library, expression string
		limit               int
		want                error
	}{
		{"", "", 1, nil},
		{" \n", "é", 4, nil},
		{" \n", "é", 3, jsruntime.ErrLimitExceeded},
		{"privateLibrary", "", 1, jsruntime.ErrLimitExceeded},
		{"", "privateExpression", 1, jsruntime.ErrLimitExceeded},
		{"", "", 0, jsruntime.ErrInvalidInput},
		{"", "", -1, jsruntime.ErrInvalidInput},
	} {
		// Whitespace and multibyte characters count without trimming or parsing.
		err := CheckCombinedSource(test.library, test.expression, test.limit)
		if test.want == nil {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "private")
		}
	}

	// Runtime adapters still hide validation-only detail behind their public category.
	limits := ConnectorMigrationLimits()
	limits.MaxSourceBytes = 1
	_, err := NewContext(nil, nil).EvaluateObjectJSONWithContext(context.Background(), "({})", limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.EqualError(t, err, jsruntime.ErrLimitExceeded.Error())
}

// TestBoundedSyntaxDoesNotExecute preserves compile-only validation for hooks
// that need connection inputs and helper functions available only at runtime.
func TestBoundedSyntaxDoesNotExecute(t *testing.T) {
	// Neither a library loop nor a hook loop can run during syntax validation.
	library, err := CompileLibrary(`for (;;) {}`)
	require.NoError(t, err)
	for _, source := range []string{
		`for (;;) {}`,
		`throw new Error("privateDetail")`,
		`({config: {set: {name: cfg.name}}, labels: {set: labels}, annotations: {set: annotations}})`,
		`var result = helper(cfg); result;`,
		`assignedWithoutDeclaration = 1; ({})`,
		`({get value() { for (;;) {} }})`,
		`new Promise(() => {})`,
		"helper(cfg);\n//# sourceMappingURL=file:///authproxy-do-not-load-source-map.json",
	} {
		require.NoError(t, library.ValidateExpressionSyntaxWithContext(context.Background(), source, ConnectorMigrationLimits()))
	}
}

// TestBoundedSyntaxSourceLimits charges raw UTF-8 library and hook bytes against
// one allowance, including whitespace retained by bounded library validation.
func TestBoundedSyntaxSourceLimits(t *testing.T) {
	for _, source := range []string{"({})", "({name: 'é'})"} {
		// Exact admission is based on bytes, not characters, and does not trim source.
		limits := ConnectorMigrationLimits()
		library, err := CompileAndValidateLibraryWithContext(context.Background(), " \n\t ", limits)
		require.NoError(t, err)
		limits.MaxSourceBytes = len(library.Source()) + len(source)
		require.NoError(t, library.ValidateExpressionSyntaxWithContext(context.Background(), source, limits))
		limits.MaxSourceBytes--
		require.ErrorIs(t, library.ValidateExpressionSyntaxWithContext(context.Background(), source, limits), jsruntime.ErrLimitExceeded)
	}

	// Oversized malformed or blank source is rejected before parsing or trimming.
	limits := ConnectorMigrationLimits()
	limits.MaxSourceBytes = 8
	for _, source := range []string{strings.Repeat(" ", 9), "privateDetail = ;"} {
		err := (*Library)(nil).ValidateExpressionSyntaxWithContext(context.Background(), source, limits)
		require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
		require.ErrorContains(t, err, "combined source exceeds 8 bytes")
		require.NotContains(t, err.Error(), "compilation time")
		require.NotContains(t, err.Error(), "privateDetail")
	}
}

// TestBoundedSyntaxDiagnostics retains actionable numeric syntax positions
// without exposing authored messages, source text, or parser errors to callers.
func TestBoundedSyntaxDiagnostics(t *testing.T) {
	for _, test := range []struct {
		source   string
		position string
	}{
		{"\n\nconst privateDetail = ;", "line 3, column 23"},
		{"let privateDetail = 1;\nlet privateDetail = 2;", "line 2, column 5"},
	} {
		// Parse and AST-compile failures both become the same safe category.
		err := (*Library)(nil).ValidateExpressionSyntaxWithContext(context.Background(), test.source, ConnectorMigrationLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		require.ErrorContains(t, err, test.position)
		require.NotContains(t, err.Error(), "privateDetail")
		require.NotContains(t, err.Error(), "expression.js")
		require.NotContains(t, err.Error(), "Unexpected")
		var parseErrors parser.ErrorList
		var compileError *goja.CompilerSyntaxError
		require.False(t, errors.As(err, &parseErrors))
		require.False(t, errors.As(err, &compileError))
	}

	// Invalid encoding and omitted statements have no parser details to expose.
	for _, source := range []string{"", " \n\t", "//\xff"} {
		err := (*Library)(nil).ValidateExpressionSyntaxWithContext(context.Background(), source, ConnectorMigrationLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		require.EqualError(t, err, jsruntime.ErrInvalidSource.Error())
	}
}

// TestBoundedSyntaxCancellation validates policy before compilation and retains
// caller cancellation over malformed source or an expired validation budget.
func TestBoundedSyntaxCancellation(t *testing.T) {
	var library *Library
	limits := ConnectorMigrationLimits()
	require.ErrorIs(t, library.ValidateExpressionSyntaxWithContext(nil, "({})", limits), jsruntime.ErrInvalidInput)
	require.ErrorIs(t, library.ValidateExpressionSyntaxWithContext(context.Background(), "({})", jsruntime.Limits{}), jsruntime.ErrInvalidInput)

	// A caller canceled before validation wins over policy and syntax errors.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, library.ValidateExpressionSyntaxWithContext(ctx, "privateDetail = ;", jsruntime.Limits{}), context.Canceled)
	ctx, cancel = context.WithDeadline(context.Background(), time.Time{})
	defer cancel()
	require.ErrorIs(t, library.ValidateExpressionSyntaxWithContext(ctx, "({})", limits), context.DeadlineExceeded)

	// Expiring the local allowance yields a limit failure, without executing source.
	limits.Timeout = time.Nanosecond
	err := library.ValidateExpressionSyntaxWithContext(context.Background(), "({})", limits)
	require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
	require.ErrorContains(t, err, "compilation time exceeded 1ns")
	require.NotContains(t, err.Error(), "combined source")
}

// TestBoundedSyntaxConcurrentReuse checks that validations share only immutable
// library source and do not initialize or mutate a reusable helper program.
func TestBoundedSyntaxConcurrentReuse(t *testing.T) {
	library, err := CompileLibrary(`throw "library must not run";`)
	require.NoError(t, err)
	results := make(chan error, 12)
	var work sync.WaitGroup
	for range cap(results) {
		work.Go(func() {
			results <- library.ValidateExpressionSyntaxWithContext(context.Background(), "helper(cfg)", ConnectorMigrationLimits())
		})
	}
	work.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}
