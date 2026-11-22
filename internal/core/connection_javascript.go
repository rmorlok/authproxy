package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// getBoundedJavascriptContext admits the raw combined source before loading the
// connector's helper library, then supplies connection input to the bounded
// evaluator. Definition/configuration failures retain their causes; authored
// compilation errors use safe runtime classifications.
func (c *connection) getBoundedJavascriptContext(
	ctx context.Context,
	expression string,
	limits jsruntime.Limits,
) (apjs.Context, error) {
	library, err := c.connector.getBoundedJavascriptLibrary(ctx, expression, limits)
	if err != nil {
		return apjs.Context{}, err
	}
	jsctx, err := c.getJavascriptContext(ctx, library)
	if contextErr := ctx.Err(); contextErr != nil {
		return apjs.Context{}, contextErr
	}
	if err != nil {
		return apjs.Context{}, fmt.Errorf("get javascript context: %w", err)
	}
	return jsctx, nil
}

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
	jsctx, err := c.getBoundedJavascriptContext(ctx, expression, limits)
	if err != nil {
		return false, err
	}
	return jsctx.EvaluateBooleanWithContext(ctx, expression, limits)
}
