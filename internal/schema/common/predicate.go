package common

import (
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
)

// Predicate defines a JavaScript condition evaluated by runtime with a
// caller-defined set of variables in scope. Runtime requires the fragment to
// return a boolean.
type Predicate struct {
	Javascript string `json:"javascript" yaml:"javascript"`
}

func (p *Predicate) Validate(vc *ValidationContext, vars map[string]any) error {
	return p.ValidateWithContext(vc, apjs.NewContext(nil, vars))
}

func (p *Predicate) ValidateWithContext(vc *ValidationContext, jsctx apjs.Context) error {
	if p == nil {
		return nil
	}
	result := &multierror.Error{}
	if strings.TrimSpace(p.Javascript) == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript is required"))
		return result.ErrorOrNil()
	}
	if _, err := jsctx.EvaluateBoolean(p.Javascript); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript must evaluate to a boolean: %v", err))
	}
	return result.ErrorOrNil()
}

// GetValue returns the boolean value by evaluating the predicate's javascript.
func (p *Predicate) GetValue(vars map[string]any) (bool, error) {
	return p.GetValueWithContext(apjs.NewContext(nil, vars))
}

// GetValueWithContext returns the boolean value by evaluating the predicate's
// javascript with a prebuilt JavaScript context.
func (p *Predicate) GetValueWithContext(jsctx apjs.Context) (bool, error) {
	return jsctx.EvaluateBoolean(p.Javascript)
}
