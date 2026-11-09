package common

import (
	"context"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// Predicate defines a JavaScript condition evaluated by runtime with a
// caller-defined set of variables in scope. Runtime requires the fragment to
// return a boolean.
type Predicate struct {
	Javascript string `json:"javascript" yaml:"javascript"`
}

// Validate checks that a present predicate returns a Boolean with the supplied
// validation variables. It uses the connector predicate limits and caller
// cancellation, copies inputs, and reports safe failures at the javascript path.
// As with other schema errors, the path wrapper retains error text, not causes.
func (p *Predicate) Validate(vc *ValidationContext, jsctx apjs.Context) error {
	if p == nil {
		return nil
	}
	result := &multierror.Error{}
	limits := apjs.ConnectorPredicateLimits()
	// Bound even the blank-source scan; oversized input goes to the evaluator's
	// source gate without scanning or parsing the entire authored string.
	if len(p.Javascript) <= limits.MaxSourceBytes && strings.TrimSpace(p.Javascript) == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript is required"))
		return result.ErrorOrNil()
	}
	if _, err := jsctx.EvaluateBooleanWithContext(vc.Context(), p.Javascript, limits); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript must evaluate to a boolean: %v", err))
	}
	return result.ErrorOrNil()
}

// GetValue returns the boolean value by evaluating the predicate's
// javascript with a prebuilt JavaScript context.
func (p *Predicate) GetValue(jsctx apjs.Context) (bool, error) {
	return jsctx.EvaluateBoolean(p.Javascript)
}

// GetValueWithContext evaluates this predicate with copied native inputs and
// explicit finite limits under the caller's cancellation. It preserves legacy
// predicate syntax while returning safe runtime errors and false on failure.
// A missing predicate is invalid here; callers decide whether omission enables
// or disables their resource before requesting evaluation.
func (p *Predicate) GetValueWithContext(
	ctx context.Context,
	jsctx apjs.Context,
	limits jsruntime.Limits,
) (bool, error) {
	if p == nil {
		return false, jsruntime.ErrInvalidInput
	}

	return jsctx.EvaluateBooleanWithContext(ctx, p.Javascript, limits)
}
