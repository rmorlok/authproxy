package toolsets

import (
	"fmt"
	"maps"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// ImportedToolDefaults supplies generation-owned administrator settings for
// every tool imported from OpenAPI or MCP. It cannot change source identity,
// schemas, executors, connection binding, or permission verbs. The importer
// applies these settings; authoring validation does not materialize tools.
type ImportedToolDefaults struct {
	// Labels and Annotations overlay permitted provider metadata by exact key.
	// Omitted keys retain provider values; empty maps add nothing, not deletions.
	// System provenance and connection identity always remain controller-owned.
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Limits supplies per-tool administrator caps. Missing fields inherit
	// runtime policy. Operator ceilings still apply, and provider metadata
	// cannot replace or loosen these limits. They are not a shared ToolSet budget.
	Limits *tools.ExecutionLimits `json:"limits,omitempty" yaml:"limits,omitempty"`
}

// Validate checks user metadata and positive limits without applying them to
// provider content. Nil and empty defaults contribute no settings. The final
// merged metadata must also be validated by the importer before publication.
func (d *ImportedToolDefaults) Validate(vc *common.ValidationContext) error {
	if d == nil {
		return nil
	}
	vc = validationContext(vc)
	var result *multierror.Error
	if err := meta.ValidateUserLabels(d.Labels); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("labels", "%v", err))
	}
	if err := meta.ValidateAnnotations(d.Annotations); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("annotations", "%v", err))
	}
	result = multierror.Append(result, d.Limits.Validate(vc.PushField("limits")))
	return result.ErrorOrNil()
}

// Clone detaches metadata maps and every optional limit pointer. Nil, empty,
// and invalid settings are preserved for subsequent validation diagnostics.
func (d *ImportedToolDefaults) Clone() *ImportedToolDefaults {
	if d == nil {
		return nil
	}
	clone := *d
	clone.Labels = maps.Clone(d.Labels)
	clone.Annotations = maps.Clone(d.Annotations)
	clone.Limits = d.Limits.Clone()
	return &clone
}

// UnmarshalJSON accepts canonical fields and rejects explicit nulls, including
// map values and limit pointers, before typed decoding erases their presence.
// A successful decode replaces the whole value; failed decoding is atomic.
func (d *ImportedToolDefaults) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, "imported tool defaults", "labels", "annotations", "limits")
	if err != nil {
		return err
	}

	var decoded ImportedToolDefaults
	for name, raw := range fields {
		switch name {
		case "labels", "annotations":
			// A pointer value distinguishes null from a valid empty string.
			var values map[string]*string
			if err := util.DecodeJSONStrict(raw, &values); err != nil {
				return fmt.Errorf("decode %s: %w", name, err)
			}
			decodedValues := make(map[string]string, len(values))
			for key, value := range values {
				if value == nil {
					return fmt.Errorf("%s[%q] must be a string, not null", name, key)
				}
				decodedValues[key] = *value
			}
			if name == "labels" {
				decoded.Labels = decodedValues
			} else {
				decoded.Annotations = decodedValues
			}
		case "limits":
			if _, err := decodeStrictObject(raw, "execution limits", jsonFieldNames(reflect.TypeOf(tools.ExecutionLimits{}))...); err != nil {
				return fmt.Errorf("limits: %w", err)
			}
			if err := util.DecodeJSONStrict(raw, &decoded.Limits); err != nil {
				return fmt.Errorf("decode limits: %w", err)
			}
		}
	}
	*d = decoded
	return nil
}

// UnmarshalYAML resolves aliases and merges through the strict JSON boundary.
func (d *ImportedToolDefaults) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return d.UnmarshalJSON(raw)
}
