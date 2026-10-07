package toolsets

import (
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolSetDefinition contains generation-owned behavior. Connection selection
// is a logical ToolSet property and deliberately lives outside this definition.
// This first contract supports explicit templates; imported source contracts
// are introduced separately before OpenAPI or MCP can be authored here.
type ToolSetDefinition struct {
	Source ToolSetSource `json:"source" yaml:"source"`
}

// ToolSetSource selects the source of a generation's tool inventory. Explicit
// is currently the only supported source. Input boundaries must use strict
// decoding so future or misspelled source kinds cannot silently disappear.
type ToolSetSource struct {
	Explicit *ExplicitSource `json:"explicit,omitempty" yaml:"explicit,omitempty"`
}

// ExplicitSource declares an entire inventory of authored templates. Tools
// must be present; an empty list deliberately defines an empty inventory.
type ExplicitSource struct {
	Tools []ToolTemplate `json:"tools" yaml:"tools"`
}

// ToolTemplate is a connection-independent recipe for one generated Tool.
// Key is an exact, stable identity within its ToolSet. Changing the key creates
// a different generated identity, while changing its display name does not.
// The controller supplies the connection reference and ownership at install.
type ToolTemplate struct {
	Key      string                `json:"key" yaml:"key"`
	Metadata *ToolTemplateMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec     tools.ToolDefinition  `json:"spec" yaml:"spec"`
}

// ToolTemplateMetadata contains only administrator-authored display metadata.
// Identity, namespace, generation, timestamps, and ownership are assigned by
// the controller and are not authorable template fields. Name is an optional
// naming stem; final generated names also incorporate stable identity.
type ToolTemplateMetadata struct {
	Name        common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Labels      map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// Validate checks the complete authored generation without compiling code or
// resolving connections. Publication performs execution compilation later.
func (d *ToolSetDefinition) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if d == nil {
		return vc.NewError("toolset definition is required")
	}
	return d.Source.Validate(vc.PushField("source"))
}

// Validate requires the supported explicit source rather than accepting an
// absent source as an implicitly empty inventory.
func (s *ToolSetSource) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if s == nil {
		return vc.NewError("toolset source is required")
	}
	return s.Explicit.Validate(vc.PushField("explicit"))
}

// Validate checks every template and rejects repeated exact source keys.
// Empty inventories are valid, but omission or null must not mean removal.
func (s *ExplicitSource) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if s == nil {
		return vc.NewError("explicit source is required")
	}
	if s.Tools == nil {
		return vc.NewErrorForField("tools", "must be an array; use [] for an empty inventory")
	}
	var result *multierror.Error
	keys := make(map[string]bool, len(s.Tools))
	for i := range s.Tools {
		template := &s.Tools[i]
		path := vc.PushField("tools").PushIndex(i)
		result = multierror.Append(result, template.Validate(path))
		if keys[template.Key] {
			result = multierror.Append(result, path.NewErrorForField("key", "must be unique within the ToolSet"))
		}
		keys[template.Key] = true
	}
	return result.ErrorOrNil()
}

// Validate checks template identity, authorable metadata, and the same complete
// definition used by standalone Tools. It never normalizes a source key.
func (t *ToolTemplate) Validate(vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if t == nil {
		return vc.NewError("tool template is required")
	}
	var result *multierror.Error
	if strings.TrimSpace(t.Key) == "" {
		result = multierror.Append(result, vc.NewErrorForField("key", "must not be empty"))
	}
	result = multierror.Append(result, t.Metadata.Validate(vc.PushField("metadata")))
	result = multierror.Append(result, t.Spec.Validate(vc.PushField("spec")))
	return result.ErrorOrNil()
}

// Validate checks optional display metadata without permitting system labels
// to be authored. Selectors use the separate read-oriented label validation.
func (m *ToolTemplateMetadata) Validate(vc *common.ValidationContext) error {
	if m == nil {
		return nil
	}
	vc = validationContext(vc)
	var result *multierror.Error
	if m.Name != "" {
		if err := m.Name.Validate(); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("name", "%v", err))
		}
	}
	if err := meta.ValidateUserLabels(m.Labels); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("labels", "%v", err))
	}
	if err := meta.ValidateAnnotations(m.Annotations); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("annotations", "%v", err))
	}
	return result.ErrorOrNil()
}

// Clone returns a detached generation definition, including raw executable and
// schema bytes. It does not serialize or validate, so malformed input can still
// be copied without losing the information needed for diagnostics.
func (d *ToolSetDefinition) Clone() *ToolSetDefinition {
	if d == nil {
		return nil
	}
	clone := *d
	if d.Source.Explicit != nil {
		clone.Source.Explicit = util.CloneValue(d.Source.Explicit)
		clone.Source.Explicit.Tools = slices.Clone(d.Source.Explicit.Tools)
		for i := range clone.Source.Explicit.Tools {
			template := &clone.Source.Explicit.Tools[i]
			template.Spec = *template.Spec.Clone()
			if template.Metadata != nil {
				template.Metadata = util.CloneValue(template.Metadata)
				template.Metadata.Labels = maps.Clone(template.Metadata.Labels)
				template.Metadata.Annotations = maps.Clone(template.Metadata.Annotations)
			}
		}
	}
	return &clone
}
