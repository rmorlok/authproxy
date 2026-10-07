package toolsets

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ToolSetKind identifies the canonical ToolSet resource in envelopes and references.
const ToolSetKind meta.Kind = "ToolSet"

// ToolSetReleaseState describes the observed lifecycle of one ToolSet generation.
// Authors may request draft or primary; active and archived are server-owned states.
type ToolSetReleaseState string

const (
	ToolSetReleaseStateDraft    ToolSetReleaseState = "draft"
	ToolSetReleaseStatePrimary  ToolSetReleaseState = "primary"
	ToolSetReleaseStateActive   ToolSetReleaseState = "active"
	ToolSetReleaseStateArchived ToolSetReleaseState = "archived"
)

// ToolSet represents a logical collection of generated Tools and one generation
// of its definition. Metadata identity and the connection selector are shared
// across generations; release intent and definition belong to this generation.
type ToolSet struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta `json:"metadata" yaml:"metadata"`
	Spec          ToolSetSpec     `json:"spec" yaml:"spec"`
	Status        *ToolSetStatus  `json:"status,omitempty" yaml:"status,omitempty"`
}

// ToolSetSpec separates logical connection membership from generation content.
// Changing ConnectionSelector reconciles membership without migrating existing
// bindings. Generation responses must project the current logical selector.
type ToolSetSpec struct {
	ConnectionSelector *ConnectionSelector `json:"connectionSelector" yaml:"connectionSelector"`
	Release            ToolSetReleaseSpec  `json:"release,omitempty" yaml:"release,omitempty"`
	Definition         ToolSetDefinition   `json:"definition" yaml:"definition"`
}

// ToolSetReleaseSpec records publication intent. Published generations retain
// primary intent even after their observed state becomes active or archived.
type ToolSetReleaseSpec struct {
	DesiredState ToolSetReleaseState `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`
}

// ToolSetStatus contains server-observed state for the addressed generation.
// A release state does not describe the generations pinned by individual connections.
type ToolSetStatus struct {
	Release ToolSetReleaseStatus `json:"release" yaml:"release"`
}

// ToolSetReleaseStatus records the server-owned lifecycle state of a generation.
type ToolSetReleaseStatus struct {
	State ToolSetReleaseState `json:"state" yaml:"state"`
}

// NewToolSet returns an empty ToolSet with the canonical API version and kind.
func NewToolSet() *ToolSet {
	return &ToolSet{TypeMeta: meta.NewTypeMeta(ToolSetKind)}
}

// NewToolSetReference returns a reference to the logical ToolSet identity.
// Callers that pin a generation must populate Generation explicitly.
func NewToolSetReference(id apid.ID) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       ToolSetKind,
		ID:         id.String(),
	}
}

// GetId returns the logical ToolSet ID, or apid.Nil if it is absent or invalid.
func (t *ToolSet) GetId() apid.ID {
	if t == nil || ValidateID(t.Metadata.ID) != nil {
		return apid.Nil
	}

	id, _ := apid.Parse(t.Metadata.ID)
	return id
}

// ValidateID checks that an identifier belongs to the ToolSet resource kind.
func ValidateID(value string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != apid.PrefixToolSet {
		return fmt.Errorf("must be a tool set id")
	}
	return nil
}

// ApplyAPICreateDefaults returns a detached initial generation with the allocated
// identity, a fallback name, draft release intent, and an explicit selector
// namespace. Missing selectors remain invalid; defaults never imply match-all.
// This helper neither publishes a generation nor synthesizes observed status.
// Configuration defaults belong to future reconciliation, not this API helper.
func (t *ToolSet) ApplyAPICreateDefaults(id apid.ID) *ToolSet {
	clone := t.Clone()
	if clone == nil {
		return nil
	}

	clone.Metadata.ID = id.String()
	clone.Metadata.Generation = 1
	if clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}
	if clone.Spec.Release.DesiredState == "" {
		clone.Spec.Release.DesiredState = ToolSetReleaseStateDraft
	}
	if clone.Spec.ConnectionSelector != nil && clone.Spec.ConnectionSelector.Namespace == "" {
		clone.Spec.ConnectionSelector.Namespace = clone.Metadata.Namespace + namespaceschema.WildcardSuffix
	}
	return clone
}

// Clone returns a detached resource snapshot, preserving nil values and copying
// logical metadata, selector labels, generation content, and observed status.
func (t *ToolSet) Clone() *ToolSet {
	if t == nil {
		return nil
	}

	clone := *t
	clone.Metadata = meta.CloneObjectMeta(t.Metadata)
	clone.Spec.ConnectionSelector = t.Spec.ConnectionSelector.Clone()
	clone.Spec.Definition = *t.Spec.Definition.Clone()
	if t.Status != nil {
		status := *t.Status
		clone.Status = &status
	}
	return &clone
}

// Validate applies configuration-file rules without choosing an omitted release
// state. Publication compilation and connection reconciliation are service work.
func (t *ToolSet) Validate(vc *common.ValidationContext) error {
	return t.ValidateFor(meta.ValidationModeConfig, vc)
}

// ValidateFor checks a ToolSet snapshot at a lifecycle boundary. Stored resources
// require resolved identity and compatible desired/observed release state. This
// validates shape, not transitions, generation uniqueness, or publication readiness.
func (t *ToolSet) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	vc = validationContext(vc)
	if t == nil {
		return vc.NewError("tool set is required")
	}

	stored := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse
	var result *multierror.Error
	result = multierror.Append(result, meta.ValidateResource(t.TypeMeta, t.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       ToolSetKind,
		RequireID:          stored,
		RequireName:        stored,
		RequireNamespace:   true,
		IDValidator:        ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}))
	if stored && t.Metadata.Generation == 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "must be greater than zero"))
	}

	result = multierror.Append(result, t.Spec.ConnectionSelector.ValidateForNamespace(
		t.Metadata.Namespace, vc.PushField("spec").PushField("connectionSelector"),
	))
	result = multierror.Append(result, t.Spec.Definition.Validate(vc.PushField("spec").PushField("definition")))

	desired := t.Spec.Release.DesiredState
	switch desired {
	case "":
		if stored {
			result = multierror.Append(result, vc.NewErrorForField("spec.release.desiredState", "is required"))
		}
	case ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary:
	default:
		result = multierror.Append(result, vc.NewErrorForField("spec.release.desiredState", "must be either draft or primary"))
	}

	result = multierror.Append(result, meta.ValidateStatus(t.Status, mode, vc))
	if t.Status == nil {
		if stored {
			result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
		}
		return result.ErrorOrNil()
	}

	observed := t.Status.Release.State
	switch observed {
	case ToolSetReleaseStateDraft, ToolSetReleaseStatePrimary, ToolSetReleaseStateActive, ToolSetReleaseStateArchived:
	default:
		result = multierror.Append(result, vc.NewErrorForField("status.release.state", "is not a recognized tool set release state"))
	}
	if stored {
		switch desired {
		case ToolSetReleaseStateDraft:
			if observed != ToolSetReleaseStateDraft {
				result = multierror.Append(result, vc.NewErrorForField("status.release.state", "must be draft when desiredState is draft"))
			}
		case ToolSetReleaseStatePrimary:
			if observed != ToolSetReleaseStatePrimary && observed != ToolSetReleaseStateActive && observed != ToolSetReleaseStateArchived {
				result = multierror.Append(result, vc.NewErrorForField("status.release.state", "must be primary, active, or archived when desiredState is primary"))
			}
		}
	}
	return result.ErrorOrNil()
}

// validationContext preserves a caller's path or supplies the document root.
func validationContext(vc *common.ValidationContext) *common.ValidationContext {
	if vc == nil {
		return &common.ValidationContext{Path: "$"}
	}
	return vc
}
