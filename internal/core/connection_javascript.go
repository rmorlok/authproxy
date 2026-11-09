package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// EvaluateJavascriptPredicate evaluates a connector condition against copied
// configuration, labels, and annotations using the shared finite predicate
// profile. It loads this connection's captured connector generation and data
// with ctx, then runs a fresh bounded VM without I/O capabilities. Callers decide
// what an omitted condition means and add their probe, step, or scope identity
// to errors. Failure always returns false.
//
// Definition and configuration failures retain their causes for retry decisions.
// Authored compile/evaluation failures expose only safe runtime classifications.
// Source is checked before the library cache can compile it, and caller
// cancellation takes precedence over errors from that noninterruptible parsing.
func (c *connection) EvaluateJavascriptPredicate(ctx context.Context, expression string) (bool, error) {
	if ctx == nil {
		return false, jsruntime.ErrInvalidInput
	}
	def, err := c.connector.getDefinitionWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("load connector definition: %w", err)
	}

	limits := apjs.ConnectorPredicateLimits()
	if len(expression) > limits.MaxSourceBytes || len(def.Javascript) > limits.MaxSourceBytes-len(expression) {
		return false, jsruntime.ErrLimitExceeded
	}

	// The contextual load populated the definition cache. Library failures
	// below can only be compile errors, not decryption or decode failures.
	library, err := c.connector.getJavascriptLibrary()
	if contextErr := ctx.Err(); contextErr != nil {
		return false, contextErr
	}
	if err != nil {
		return false, jsruntime.ErrInvalidSource
	}

	jsctx, err := c.getJavascriptContext(ctx, library)
	if err != nil {
		return false, fmt.Errorf("get javascript context: %w", err)
	}
	return jsctx.EvaluateBooleanWithContext(ctx, expression, limits)
}
