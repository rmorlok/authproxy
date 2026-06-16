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
	if p == nil {
		return nil
	}
	result := &multierror.Error{}
	if strings.TrimSpace(p.Javascript) == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript is required"))
		return result.ErrorOrNil()
	}
	if _, err := apjs.EvaluateBoolean(p.Javascript, vars); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "javascript must evaluate to a boolean: %v", err))
	}
	return result.ErrorOrNil()
}
