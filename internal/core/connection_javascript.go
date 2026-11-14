package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
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
	limits := apjs.ConnectorPredicateLimits()
	library, err := c.connector.getBoundedJavascriptLibrary(ctx, expression, limits)
	if err != nil {
		return false, err
	}

	jsctx, err := c.getJavascriptContext(ctx, library)
	if err != nil {
		return false, fmt.Errorf("get javascript context: %w", err)
	}
	return jsctx.EvaluateBooleanWithContext(ctx, expression, limits)
}
