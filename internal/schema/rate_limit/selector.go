package rate_limit

import (
	"net/http"
	"regexp"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// DefaultRequestTypes is applied when a Selector leaves RequestTypes nil
// (i.e., the field is omitted from the input). An explicit empty list is
// rejected at validation rather than treated as "default".
func DefaultRequestTypes() []common.RequestType {
	return []common.RequestType{common.RequestTypeProxy, common.RequestTypeProbe}
}

// PathMatchKind selects how PathMatch.Value is interpreted.
type PathMatchKind string

const (
	PathMatchKindPrefix PathMatchKind = "prefix"
	PathMatchKindGlob   PathMatchKind = "glob"
	PathMatchKindRegex  PathMatchKind = "regex"
)

// IsValidPathMatchKind reports whether k is a recognised PathMatchKind.
func IsValidPathMatchKind(k PathMatchKind) bool {
	switch k {
	case PathMatchKindPrefix, PathMatchKindGlob, PathMatchKindRegex:
		return true
	default:
		return false
	}
}

// PathMatch matches a request's final upstream URL path.
type PathMatch struct {
	Kind  PathMatchKind `json:"kind" yaml:"kind"`
	Value string        `json:"value" yaml:"value"`
}

// Validate ensures Kind is recognised, Value is non-empty, and — for regex
// kind — Value compiles.
func (p *PathMatch) Validate(vc *common.ValidationContext) error {
	if p == nil {
		return nil
	}
	result := &multierror.Error{}

	if !IsValidPathMatchKind(p.Kind) {
		result = multierror.Append(result, vc.NewErrorfForField("kind", "invalid kind %q", string(p.Kind)))
	}

	if p.Value == "" {
		result = multierror.Append(result, vc.NewErrorForField("value", "must not be empty"))
	} else if p.Kind == PathMatchKindRegex {
		if _, err := regexp.Compile(p.Value); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("value", "invalid regex: %s", err.Error()))
		}
	}

	return result.ErrorOrNil()
}

// Selector matches proxy/probe requests against a rule's criteria. All
// non-empty clauses are combined with logical AND.
type Selector struct {
	// LabelSelector is a Kubernetes-style selector string evaluated against
	// the per-request label snapshot. Parsing is the runtime layer's
	// responsibility; this schema only ensures it is non-pathological.
	LabelSelector string `json:"label_selector,omitempty" yaml:"label_selector,omitempty"`

	// Methods restricts the rule to specific HTTP verbs. Empty / nil means any.
	Methods []string `json:"methods,omitempty" yaml:"methods,omitempty"`

	// PathMatch restricts the rule to a path on the final upstream URL.
	PathMatch *PathMatch `json:"path_match,omitempty" yaml:"path_match,omitempty"`

	// RequestTypes restricts the rule to specific request types. nil means
	// "use DefaultRequestTypes()". An explicit empty slice is rejected at
	// validation so an operator can't accidentally create an inert rule.
	RequestTypes []common.RequestType `json:"request_types,omitempty" yaml:"request_types,omitempty"`
}

// EffectiveRequestTypes returns RequestTypes when set, otherwise the default.
func (s *Selector) EffectiveRequestTypes() []common.RequestType {
	if s == nil || s.RequestTypes == nil {
		return DefaultRequestTypes()
	}
	return s.RequestTypes
}

// validHTTPMethods is the set of method tokens accepted in Selector.Methods.
// Limiting to RFC 7231 + WebDAV-ish names keeps typos from silently producing
// rules that match nothing.
var validHTTPMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodOptions: true,
	http.MethodConnect: true,
	http.MethodTrace:   true,
}

// Validate runs the selector's validation rules.
func (s *Selector) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	for i, m := range s.Methods {
		if !validHTTPMethods[m] {
			result = multierror.Append(result, vc.PushField("methods").PushIndex(i).NewErrorf("unknown HTTP method %q", m))
		}
	}

	if err := s.PathMatch.Validate(vc.PushField("path_match")); err != nil {
		result = multierror.Append(result, err)
	}

	// nil = "use default"; explicit empty = configuration mistake.
	if s.RequestTypes != nil && len(s.RequestTypes) == 0 {
		result = multierror.Append(result, vc.NewErrorForField("request_types", "must not be an empty list; omit the field to use the default"))
	}
	for i, t := range s.RequestTypes {
		if !common.IsValidRequestType(t) {
			result = multierror.Append(result, vc.PushField("request_types").PushIndex(i).NewErrorf("unknown request type %q", string(t)))
		}
	}

	return result.ErrorOrNil()
}
