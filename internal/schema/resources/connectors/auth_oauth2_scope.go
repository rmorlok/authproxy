package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"gopkg.in/yaml.v3"
)

type Scope struct {
	Id       string            `json:"id" yaml:"id"`
	If       *common.Predicate `json:"if,omitempty" yaml:"if,omitempty"`
	Required *ScopeRequired    `json:"required,omitempty" yaml:"required,omitempty"`
	Reason   string            `json:"reason" yaml:"reason"`
}

func (s *Scope) IsRequired(jsctx apjs.Context) (bool, error) {
	// If the attribute is not specified, scopes default to required.
	if s == nil || s.Required == nil {
		// If unspecified, assume required.
		return true, nil
	}

	return s.Required.IsRequired(jsctx)
}

func (s *Scope) Validate(vc *common.ValidationContext) error {
	return s.ValidateWithJavascript(vc, nil)
}

func (s *Scope) ValidateWithJavascript(vc *common.ValidationContext, library *apjs.Library) error {
	if s == nil {
		return nil
	}
	result := &multierror.Error{}
	jsctx := connectorPredicateValidationContext(library)
	if err := s.If.Validate(vc.PushField("if"), jsctx); err != nil {
		result = multierror.Append(result, err)
	}
	if err := s.Required.ValidateWithJavascript(vc.PushField("required"), library); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (s Scope) Clone() Scope {
	clone := s
	if s.If != nil {
		p := *s.If
		clone.If = &p
	}
	if s.Required != nil {
		clone.Required = s.Required.Clone()
	}
	return clone
}

// ScopeRequired preserves the existing `required: true|false` shape while
// allowing a dynamic predicate object at the same YAML key. It does this by
// implementing custom JSON/YAML marshalling and unmarshalling.
type ScopeRequired struct {
	Bool      *bool             `json:"-" yaml:"-"`
	Predicate *common.Predicate `json:"-" yaml:"-"`
}

func NewScopeRequiredBool(value bool) *ScopeRequired {
	return &ScopeRequired{Bool: &value}
}

func NewScopeRequiredPredicate(predicate *common.Predicate) *ScopeRequired {
	return &ScopeRequired{Predicate: predicate}
}

func (r *ScopeRequired) IsRequired(jsctx apjs.Context) (bool, error) {
	if r == nil {
		return true, nil
	}

	if r.Bool != nil {
		return *r.Bool, nil
	}

	if r.Predicate == nil {
		return false, fmt.Errorf("required must be a boolean or predicate object")
	}

	return r.Predicate.GetValue(jsctx)
}

func (r *ScopeRequired) Clone() *ScopeRequired {
	if r == nil {
		return nil
	}
	clone := &ScopeRequired{}
	if r.Bool != nil {
		b := *r.Bool
		clone.Bool = &b
	}
	if r.Predicate != nil {
		p := *r.Predicate
		clone.Predicate = &p
	}
	return clone
}

func (r *ScopeRequired) Validate(vc *common.ValidationContext) error {
	return r.ValidateWithJavascript(vc, nil)
}

func (r *ScopeRequired) ValidateWithJavascript(vc *common.ValidationContext, library *apjs.Library) error {
	// The scope will default to required.
	if r == nil {
		return nil
	}

	if r.Bool != nil && r.Predicate != nil {
		return vc.NewError("required must be a boolean or predicate object; cannot be both")
	}

	if r.Bool == nil && r.Predicate == nil {
		return vc.NewError("required must be a boolean or predicate object")
	}

	if r.Predicate != nil {
		return r.Predicate.Validate(vc, connectorPredicateValidationContext(library))
	}

	return nil
}

func (r ScopeRequired) MarshalJSON() ([]byte, error) {
	if r.Predicate != nil {
		return json.Marshal(r.Predicate)
	}
	if r.Bool != nil {
		return json.Marshal(*r.Bool)
	}
	return []byte("null"), nil
}

func (r *ScopeRequired) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("required must be a boolean or predicate object")
	}

	var boolValue bool
	if err := json.Unmarshal(data, &boolValue); err == nil {
		r.Bool = &boolValue
		r.Predicate = nil
		return nil
	}

	var predicate common.Predicate
	if err := json.Unmarshal(data, &predicate); err == nil {
		r.Bool = nil
		r.Predicate = &predicate
		return nil
	}

	return fmt.Errorf("required must be a boolean or predicate object")
}

func (r ScopeRequired) MarshalYAML() (any, error) {
	if r.Predicate != nil {
		return r.Predicate, nil
	}
	if r.Bool != nil {
		return *r.Bool, nil
	}
	return nil, nil
}

func (r *ScopeRequired) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Tag == "!!null" {
		return fmt.Errorf("required must be a boolean or predicate object")
	}

	var boolValue bool
	if err := value.Decode(&boolValue); err == nil {
		r.Bool = &boolValue
		r.Predicate = nil
		return nil
	}

	var predicate common.Predicate
	if err := value.Decode(&predicate); err == nil {
		r.Bool = nil
		r.Predicate = &predicate
		return nil
	}

	return fmt.Errorf("required must be a boolean or predicate object")
}
