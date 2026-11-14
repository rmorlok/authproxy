package apjs

import (
	"context"
	"strings"
	"unicode/utf8"

	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// ValidateExpressionSyntaxWithContext compiles a non-strict connector statement
// list without executing it or its helper library. Runtime names and helpers may
// be unresolved: this validates syntax, not the result or runtime behavior. A nil
// library is supported; otherwise its source shares the expression's byte limit.
// The library must already have been compiled under its own source limit.
//
// A context and positive limits are required. Parsing is not interruptible, so
// source bytes bound the admitted work and cancellation/timeout are checked
// before and after compilation. Caller cancellation takes precedence over the
// policy timeout. Errors retain only a safe category and a one-based syntax
// position when available, never source, filenames, or raw parser diagnostics.
// No VM or host capability is created, and source maps remain disabled.
func (l *Library) ValidateExpressionSyntaxWithContext(
	ctx context.Context,
	expression string,
	limits jsruntime.Limits,
) error {
	if ctx == nil {
		return jsruntime.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := limits.Validate(); err != nil {
		return err
	}
	if len(expression) > limits.MaxSourceBytes || len(l.Source()) > limits.MaxSourceBytes-len(expression) {
		return jsruntime.ErrLimitExceeded
	}
	if !utf8.ValidString(expression) || strings.TrimSpace(expression) == "" {
		return jsruntime.ErrInvalidSource
	}

	parseCtx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	var compileErr error
	if parseCtx.Err() == nil {
		// Compile through the shared parser so both syntax and AST compilation
		// errors are caught without evaluating cfg, helpers, loops, or throws.
		_, compileErr = compileSource("expression.js", expression)
	}

	// Observe the child before the parent: inherited cancellation must already
	// be visible in the caller, including when it arrived during compilation.
	parseErr := parseCtx.Err()
	if err := ctx.Err(); err != nil {
		return err
	}
	if parseErr != nil {
		return jsruntime.ErrLimitExceeded
	}
	if compileErr != nil {
		return compileValidationError(compileErr)
	}
	return nil
}
