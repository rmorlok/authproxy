package tools

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ToolPatch updates authored state on a standalone Tool. Metadata and Spec
// must be supplied as objects, even for an otherwise empty patch. Status is
// server-owned and can never advance a revision through this contract.
type ToolPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *ToolSpecPatch        `json:"spec" yaml:"spec"`
	Status        *ToolStatus           `json:"status,omitempty" yaml:"status,omitempty"`
}

// ToolSpecPatch distinguishes omission from replacement and explicit clearing.
// Supplied values replace an entire field, including schemas and HTTP plans;
// nested maps are never recursively merged. Use the Set methods to construct
// explicit null clears in Go. Required fields cannot be cleared.
type ToolSpecPatch struct {
	ConnectionRef *meta.ObjectReference `json:"connectionRef,omitempty" yaml:"connectionRef,omitempty"`
	Description   *string               `json:"description,omitempty" yaml:"description,omitempty"`
	Verbs         *[]string             `json:"verbs,omitempty" yaml:"verbs,omitempty"`
	InputSchema   common.RawJSON        `json:"inputSchema,omitempty" yaml:"inputSchema,omitempty"`
	OutputSchema  common.RawJSON        `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty"`
	Hints         *BehavioralHints      `json:"hints,omitempty" yaml:"hints,omitempty"`
	Limits        *ExecutionLimits      `json:"limits,omitempty" yaml:"limits,omitempty"`
	ProxyHTTP     *ProxyHTTP            `json:"proxyHttp,omitempty" yaml:"proxyHttp,omitempty"`
	Javascript    *string               `json:"javascript,omitempty" yaml:"javascript,omitempty"`

	present map[string]bool
}

// NewToolPatch initializes a canonical update envelope with the required empty
// metadata and spec objects, ready for callers to add their desired changes.
func NewToolPatch() *ToolPatch {
	return &ToolPatch{
		TypeMeta: meta.NewTypeMeta(ToolKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &ToolSpecPatch{},
	}
}

// ValidateFor checks the update envelope and required-field presence. Complete
// definition validation happens after merging, so an executor can be cleared
// and its replacement supplied together without rejecting intermediate state.
func (p *ToolPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if p == nil {
		return vc.NewError("tool patch is required")
	}
	var result *multierror.Error
	if mode != meta.ValidationModeUpdate {
		result = multierror.Append(result, vc.NewError("tool patches require update validation mode"))
	}
	result = multierror.Append(result, meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, ToolKind, vc))
	if p.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else {
		result = multierror.Append(result, meta.ValidateObjectMetaPatch(*p.Metadata, meta.ValidationOptions{
			Mode: mode, Path: vc, IDValidator: ValidateID, NamespaceValidator: nschema.ValidatePath,
		}))
		if p.Metadata.Generation != nil {
			result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to tools"))
		}
	}
	if p.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else {
		path := vc.PushField("spec")
		for _, field := range []struct {
			name     string
			null     bool
			nonempty bool
		}{
			{"connectionRef", p.Spec.ConnectionRef == nil, p.Spec.ConnectionRef != nil},
			{"description", p.Spec.Description == nil, p.Spec.Description != nil},
			{"verbs", p.Spec.Verbs == nil || *p.Spec.Verbs == nil, p.Spec.Verbs != nil},
			{"inputSchema", p.Spec.InputSchema == nil || bytes.Equal(bytes.TrimSpace(p.Spec.InputSchema), []byte("null")), p.Spec.InputSchema != nil},
		} {
			if p.Spec.has(field.name, field.nonempty) && field.null {
				result = multierror.Append(result, path.NewErrorForField(field.name, "must not be null"))
			}
		}
		if p.Spec.ConnectionRef != nil {
			result = multierror.Append(result, ValidateConnectionReference(*p.Spec.ConnectionRef, "", path.PushField("connectionRef")))
		}
	}
	result = multierror.Append(result, meta.ValidateStatus(p.Status, mode, vc))
	return result.ErrorOrNil()
}

// ApplyTo returns a detached candidate after validating the complete merged
// definition and immutable metadata. Neither the current Tool nor any patch
// value is mutated. The current status and revision remain server-owned.
func (p *ToolPatch) ApplyTo(current *Tool, vc *common.ValidationContext) (*Tool, error) {
	vc = validationContext(vc)
	if current == nil {
		return nil, vc.NewError("current tool is required")
	}
	if err := p.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	// Only replace top-level fields before cloning. This shallow candidate
	// does not mutate nested values borrowed from either input; one final
	// clone detaches both inherited state and every supplied replacement.
	updated := *current
	updated.Metadata = meta.ApplyObjectMetaPatch(current.Metadata, *p.Metadata)
	if p.Spec.ConnectionRef != nil {
		updated.Spec.ConnectionRef = *p.Spec.ConnectionRef
	}
	if p.Spec.Description != nil {
		updated.Spec.Description = *p.Spec.Description
	}
	if p.Spec.Verbs != nil {
		updated.Spec.Verbs = *p.Spec.Verbs
	}
	if p.Spec.has("inputSchema", p.Spec.InputSchema != nil) {
		updated.Spec.InputSchema = p.Spec.InputSchema
	}
	if p.Spec.has("outputSchema", p.Spec.OutputSchema != nil) {
		updated.Spec.OutputSchema = p.Spec.OutputSchema
		if bytes.Equal(bytes.TrimSpace(updated.Spec.OutputSchema), []byte("null")) {
			updated.Spec.OutputSchema = nil
		}
	}
	if p.Spec.has("hints", p.Spec.Hints != nil) {
		updated.Spec.Hints = p.Spec.Hints
	}
	if p.Spec.has("limits", p.Spec.Limits != nil) {
		updated.Spec.Limits = p.Spec.Limits
	}
	if p.Spec.has("proxyHttp", p.Spec.ProxyHTTP != nil) {
		updated.Spec.ProxyHTTP = p.Spec.ProxyHTTP
	}
	if p.Spec.has("javascript", p.Spec.Javascript != nil) {
		updated.Spec.Javascript = p.Spec.Javascript
	}
	candidate := updated.Clone()
	if err := ValidateUpdate(current, candidate, vc); err != nil {
		return nil, err
	}
	if err := candidate.Spec.ValidateForNamespace(candidate.Metadata.Namespace, vc.PushField("spec")); err != nil {
		return nil, err
	}
	return candidate, nil
}

// MarshalJSON preserves omitted fields and explicit nulls for clearing fields.
func (p ToolSpecPatch) MarshalJSON() ([]byte, error) {
	fields := make(map[string]any)
	for _, field := range []struct {
		name     string
		value    any
		nonempty bool
	}{
		{"connectionRef", p.ConnectionRef, p.ConnectionRef != nil},
		{"description", p.Description, p.Description != nil},
		{"verbs", p.Verbs, p.Verbs != nil},
		{"inputSchema", p.InputSchema, p.InputSchema != nil},
		{"outputSchema", p.OutputSchema, p.OutputSchema != nil},
		{"hints", p.Hints, p.Hints != nil},
		{"limits", p.Limits, p.Limits != nil},
		{"proxyHttp", p.ProxyHTTP, p.ProxyHTTP != nil},
		{"javascript", p.Javascript, p.Javascript != nil},
	} {
		if p.has(field.name, field.nonempty) {
			fields[field.name] = field.value
		}
	}
	return json.Marshal(fields)
}

// UnmarshalJSON decodes canonical field names strictly and retains null
// presence. Assignment is atomic: failed decoding leaves the receiver intact.
func (p *ToolSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := util.DecodeJSONStrict(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("tool spec patch must be an object")
	}
	decoded := ToolSpecPatch{present: make(map[string]bool, len(fields))}
	for name, raw := range fields {
		var destination any
		switch name {
		case "connectionRef":
			destination = &decoded.ConnectionRef
		case "description":
			destination = &decoded.Description
		case "verbs":
			destination = &decoded.Verbs
		case "inputSchema":
			destination = &decoded.InputSchema
		case "outputSchema":
			destination = &decoded.OutputSchema
		case "hints":
			destination = &decoded.Hints
		case "limits":
			destination = &decoded.Limits
		case "proxyHttp":
			destination = &decoded.ProxyHTTP
		case "javascript":
			destination = &decoded.Javascript
		default:
			return fmt.Errorf("unknown tool spec patch field %q", name)
		}
		if err := util.DecodeJSONStrict(raw, destination); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
		decoded.present[name] = true
	}
	*p = decoded
	return nil
}

// MarshalYAML uses the same presence-aware representation as JSON, retaining
// structured schemas and exact numeric values in emitted YAML.
func (p ToolSpecPatch) MarshalYAML() (any, error) {
	raw, err := p.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return common.RawJSON(raw).MarshalYAML()
}

// UnmarshalYAML resolves aliases and merges before using JSON presence rules.
// RawJSON intentionally treats provider schemas and request bodies as data.
func (p *ToolSpecPatch) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return p.UnmarshalJSON(raw)
}

// has recognizes a decoded explicit null as well as non-nil Go field values.
func (p *ToolSpecPatch) has(field string, nonempty bool) bool {
	return p != nil && (p.present[field] || nonempty)
}

// markPresent records a Go-authored field, including an explicit nil clear.
func (p *ToolSpecPatch) markPresent(field string) {
	if p.present == nil {
		p.present = make(map[string]bool)
	}
	p.present[field] = true
}

// SetOutputSchema replaces the success schema; nil clears its validation.
func (p *ToolSpecPatch) SetOutputSchema(value common.RawJSON) {
	if p != nil {
		p.OutputSchema = value
		p.markPresent("outputSchema")
	}
}

// SetHints replaces behavioral hints; nil clears every hint.
func (p *ToolSpecPatch) SetHints(value *BehavioralHints) {
	if p != nil {
		p.Hints = value
		p.markPresent("hints")
	}
}

// SetLimits replaces authored limits; nil restores inherited runtime policy.
func (p *ToolSpecPatch) SetLimits(value *ExecutionLimits) {
	if p != nil {
		p.Limits = value
		p.markPresent("limits")
	}
}

// SetProxyHTTP replaces or clears the HTTP executor. Clearing it requires a
// JavaScript executor in the complete merged definition.
func (p *ToolSpecPatch) SetProxyHTTP(value *ProxyHTTP) {
	if p != nil {
		p.ProxyHTTP = value
		p.markPresent("proxyHttp")
	}
}

// SetJavascript replaces or clears the JavaScript executor. Clearing it
// requires an HTTP executor in the complete merged definition.
func (p *ToolSpecPatch) SetJavascript(value *string) {
	if p != nil {
		p.Javascript = value
		p.markPresent("javascript")
	}
}
