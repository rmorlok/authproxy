package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"gopkg.in/yaml.v3"
)

type Scope struct {
	Id       string            `json:"id" yaml:"id"`
	If       *common.Predicate `json:"if,omitempty" yaml:"if,omitempty"`
	Required *ScopeRequired    `json:"required,omitempty" yaml:"required,omitempty"`
	Reason   string            `json:"reason" yaml:"reason"`
}

func (s *Scope) IsRequired() bool {
	if s == nil || s.Required == nil {
		// If unspecified, assume required.
		return true
	}

	return s.Required.IsRequired()
}

func (s *Scope) Validate(vc *common.ValidationContext) error {
	if s == nil {
		return nil
	}
	result := &multierror.Error{}
	if err := s.If.Validate(vc.PushField("if"), connectorPredicateValidationVars()); err != nil {
		result = multierror.Append(result, err)
	}
	if err := s.Required.Validate(vc.PushField("required")); err != nil {
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
// allowing a dynamic predicate object at the same YAML key.
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

func (r *ScopeRequired) IsRequired() bool {
	if r == nil || r.Bool == nil {
		// Runtime evaluates Predicate in the OAuth runtime work. Until then,
		// preserve the historical required-by-default behavior.
		return true
	}
	return *r.Bool
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
	if r == nil || r.Predicate == nil {
		return nil
	}
	return r.Predicate.Validate(vc, connectorPredicateValidationVars())
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
