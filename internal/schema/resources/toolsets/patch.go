package toolsets

import (
	"maps"
	"reflect"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// ToolSetPatch updates logical metadata and selection or generation-owned
// desired state. Metadata and Spec are required objects, even for an empty
// patch. Endpoint selection and draft/publication rules belong to generation
// policy; this contract never changes server-observed status.
type ToolSetPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *ToolSetSpecPatch     `json:"spec" yaml:"spec"`
	Status        *ToolSetStatus        `json:"status,omitempty" yaml:"status,omitempty"`
}

// ToolSetSpecPatch replaces complete selectors and definitions. Release alone
// is partial: an empty release object leaves desired state unchanged. Presence
// flags preserve invalid explicit nulls for validation and reserialization.
type ToolSetSpecPatch struct {
	ConnectionSelector *ConnectionSelector      `json:"connectionSelector,omitempty" yaml:"connectionSelector,omitempty"`
	Release            *ToolSetReleaseSpecPatch `json:"release,omitempty" yaml:"release,omitempty"`
	Definition         *ToolSetDefinition       `json:"definition,omitempty" yaml:"definition,omitempty"`

	connectionSelectorPresent bool
	releasePresent            bool
	definitionPresent         bool
}

// ToolSetReleaseSpecPatch requests draft or primary without mutating observed
// release state. Omission is distinct from an invalid explicit null or empty string.
type ToolSetReleaseSpecPatch struct {
	DesiredState *ToolSetReleaseState `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`

	desiredStatePresent bool
}

// NewToolSetPatch initializes the canonical envelope and required empty objects.
func NewToolSetPatch() *ToolSetPatch {
	return &ToolSetPatch{
		TypeMeta: meta.NewTypeMeta(ToolSetKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &ToolSetSpecPatch{},
	}
}

// HasConnectionSelector reports an authored replacement, including explicit null.
func (p *ToolSetSpecPatch) HasConnectionSelector() bool {
	return p != nil && (p.connectionSelectorPresent || p.ConnectionSelector != nil)
}

// HasRelease reports a supplied release object, including explicit null or {}.
func (p *ToolSetSpecPatch) HasRelease() bool {
	return p != nil && (p.releasePresent || p.Release != nil)
}

// HasDefinition reports a complete definition replacement, including explicit null.
func (p *ToolSetSpecPatch) HasDefinition() bool {
	return p != nil && (p.definitionPresent || p.Definition != nil)
}

// HasDesiredState reports supplied publication intent, including explicit null.
func (p *ToolSetReleaseSpecPatch) HasDesiredState() bool {
	return p != nil && (p.desiredStatePresent || p.DesiredState != nil)
}

// ValidateFor checks the update envelope and supplied values without needing a
// stored ToolSet. Selector syntax is checked against root here; ApplyTo checks
// the replacement against the actual immutable owner namespace.
func (p *ToolSetPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if p == nil {
		return vc.NewError("tool set patch is required")
	}

	var result *multierror.Error
	if mode != meta.ValidationModeUpdate {
		result = multierror.Append(result, vc.NewError("tool set patches require update validation mode"))
	}

	result = multierror.Append(result, meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, ToolSetKind, vc))
	if p.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else {
		result = multierror.Append(result, meta.ValidateObjectMetaPatch(*p.Metadata, meta.ValidationOptions{
			Mode: mode, Path: vc, IDValidator: ValidateID, NamespaceValidator: nschema.ValidatePath,
		}))
	}

	if p.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else {
		result = multierror.Append(result, p.Spec.validate(vc.PushField("spec")))
	}

	result = multierror.Append(result, meta.ValidateStatus(p.Status, mode, vc))
	return result.ErrorOrNil()
}

// validate rejects clearing required spec values and validates each complete
// replacement. It permits release:{} because that carries no desired-state edit.
func (p *ToolSetSpecPatch) validate(vc *common.ValidationContext) error {
	var result *multierror.Error

	if p.HasConnectionSelector() {
		if p.ConnectionSelector == nil {
			result = multierror.Append(result, vc.NewErrorForField("connectionSelector", "must not be null"))
		} else {
			result = multierror.Append(result, p.ConnectionSelector.ValidateForNamespace(nschema.Root, vc.PushField("connectionSelector")))
		}
	}

	if p.HasDefinition() {
		if p.Definition == nil {
			result = multierror.Append(result, vc.NewErrorForField("definition", "must not be null"))
		} else {
			result = multierror.Append(result, p.Definition.Validate(vc.PushField("definition")))
		}
	}

	if p.HasRelease() {
		if p.Release == nil {
			result = multierror.Append(result, vc.NewErrorForField("release", "must not be null"))
		} else if p.Release.HasDesiredState() {
			if p.Release.DesiredState == nil {
				result = multierror.Append(result, vc.NewErrorForField("release.desiredState", "must not be null"))
			} else if *p.Release.DesiredState != ToolSetReleaseStateDraft && *p.Release.DesiredState != ToolSetReleaseStatePrimary {
				result = multierror.Append(result, vc.NewErrorForField("release.desiredState", "must be either draft or primary"))
			}
		}
	}
	
	return result.ErrorOrNil()
}

// ApplyTo returns a detached candidate after merging authored fields and
// validating the complete selector and definition. Neither input is mutated.
// Status stays unchanged even when primary is requested for an observed draft;
// publication policy and the service must resolve that pending transition.
func (p *ToolSetPatch) ApplyTo(current *ToolSet, vc *common.ValidationContext) (*ToolSet, error) {
	vc = validationContext(vc)
	if current == nil {
		return nil, vc.NewError("current tool set is required")
	}
	if err := p.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	// Borrow only for this shallow overlay, then detach inherited and supplied
	// state together. A selector's omitted namespace deliberately restores the
	// owner-subtree default instead of inheriting the previous selector scope.
	updated := *current
	updated.Metadata = meta.ApplyObjectMetaPatch(current.Metadata, *p.Metadata)
	if p.Spec.ConnectionSelector != nil {
		updated.Spec.ConnectionSelector = p.Spec.ConnectionSelector
	}
	if p.Spec.Definition != nil {
		updated.Spec.Definition = *p.Spec.Definition
	}
	if p.Spec.Release != nil && p.Spec.Release.DesiredState != nil {
		updated.Spec.Release.DesiredState = *p.Spec.Release.DesiredState
	}
	candidate := updated.Clone()
	var result *multierror.Error
	result = multierror.Append(result, ValidateUpdate(current, candidate, vc))
	result = multierror.Append(result, candidate.Spec.ConnectionSelector.ValidateForNamespace(candidate.Metadata.Namespace, vc.PushField("spec").PushField("connectionSelector")))
	result = multierror.Append(result, candidate.Spec.Definition.Validate(vc.PushField("spec").PushField("definition")))
	switch candidate.Spec.Release.DesiredState {
	case "", ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary:
	default:
		result = multierror.Append(result, vc.NewErrorForField("spec.release.desiredState", "must be either draft or primary"))
	}
	if err := result.ErrorOrNil(); err != nil {
		return nil, err
	}
	return candidate, nil
}

// ValidateUpdate enforces stable logical identity and unchanged observed state.
// Definition editability and selector writes to explicit generations depend on
// endpoint context and are intentionally enforced by generation policy instead.
func ValidateUpdate(before, after *ToolSet, vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if before == nil || after == nil {
		return vc.NewError("before and after tool sets are required")
	}
	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc))
	result = multierror.Append(result, meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{ImmutableNamespace: true}, vc))
	if !util.EqualTimePointers(before.Metadata.UpdatedAt, after.Metadata.UpdatedAt) {
		result = multierror.Append(result, vc.NewErrorForField("metadata.updatedAt", "is server-owned and must remain unchanged"))
	}
	if !reflect.DeepEqual(before.Status, after.Status) {
		result = multierror.Append(result, vc.NewErrorForField("status", "is server-owned and must remain unchanged"))
	}
	return result.ErrorOrNil()
}

// Clone returns a fully detached patch without serializing or validating it.
// Missing fields, empty maps, and invalid null-presence flags are preserved so
// generation planning cannot mutate inputs or hide authoring diagnostics.
func (p *ToolSetPatch) Clone() *ToolSetPatch {
	if p == nil {
		return nil
	}
	clone := *p
	clone.Metadata = cloneMetadataPatch(p.Metadata)
	clone.Status = util.CloneValue(p.Status)
	if p.Spec != nil {
		clone.Spec = util.CloneValue(p.Spec)
		clone.Spec.ConnectionSelector = p.Spec.ConnectionSelector.Clone()
		clone.Spec.Definition = p.Spec.Definition.Clone()
		clone.Spec.Release = util.CloneValue(p.Spec.Release)
		if clone.Spec.Release != nil {
			clone.Spec.Release.DesiredState = util.CloneValue(p.Spec.Release.DesiredState)
		}
	}
	return &clone
}

// cloneMetadataPatch preserves pointer presence while copying maps, scalar
// pointers, and timestamps, including malformed server-owned field writes.
func cloneMetadataPatch(value *meta.ObjectMetaPatch) *meta.ObjectMetaPatch {
	if value == nil {
		return nil
	}
	clone := *value
	clone.ID = util.CloneValue(value.ID)
	clone.Name = util.CloneValue(value.Name)
	clone.Namespace = util.CloneValue(value.Namespace)
	clone.Generation = util.CloneValue(value.Generation)
	clone.CreatedAt = util.CloneValue(value.CreatedAt)
	clone.UpdatedAt = util.CloneValue(value.UpdatedAt)
	if value.Labels != nil {
		labels := maps.Clone(*value.Labels)
		clone.Labels = &labels
	}
	if value.Annotations != nil {
		annotations := maps.Clone(*value.Annotations)
		clone.Annotations = &annotations
	}
	return &clone
}
