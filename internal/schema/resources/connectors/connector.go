package connectors

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const ConnectorKind meta.Kind = "Connector"

type ConnectorReleaseState string

const (
	ConnectorReleaseStateDraft    ConnectorReleaseState = "draft"
	ConnectorReleaseStatePrimary  ConnectorReleaseState = "primary"
	ConnectorReleaseStateActive   ConnectorReleaseState = "active"
	ConnectorReleaseStateArchived ConnectorReleaseState = "archived"
)

// Connector is the versioned Kubernetes-style resource for a connector.
// Metadata identifies the logical connector and generation; Spec.Definition
// contains provider behavior; Status contains server-observed release state.
type Connector struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta  `json:"metadata" yaml:"metadata"`
	Spec          ConnectorSpec    `json:"spec" yaml:"spec"`
	Status        *ConnectorStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type ConnectorSpec struct {
	Release    ConnectorReleaseSpec `json:"release,omitempty" yaml:"release,omitempty"`
	Definition ConnectorDefinition  `json:"definition" yaml:"definition"`
}

// ConnectorReleaseSpec expresses the desired release state for this
// generation. Config may request draft or primary; active and archived are
// observed lifecycle states and therefore appear only in status.
type ConnectorReleaseSpec struct {
	DesiredState ConnectorReleaseState `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`
}

type ConnectorStatus struct {
	Release ConnectorReleaseStatus `json:"release" yaml:"release"`
}

type ConnectorReleaseStatus struct {
	State ConnectorReleaseState `json:"state" yaml:"state"`
}

// ConnectorPatch is a presence-aware update to one Connector generation.
// Metadata and Spec are required objects so a malformed partial envelope does
// not silently bypass resource validation.
type ConnectorPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *ConnectorSpecPatch   `json:"spec" yaml:"spec"`
	Status        *ConnectorStatus      `json:"status,omitempty" yaml:"status,omitempty"`
}

// ConnectorSpecPatch contains mutable desired-state updates. Release and
// definition use presence tracking so explicit null values are rejected rather
// than interpreted as omitted fields.
type ConnectorSpecPatch struct {
	Release    *ConnectorReleaseSpecPatch `json:"release,omitempty" yaml:"release,omitempty"`
	Definition *ConnectorDefinition       `json:"definition,omitempty" yaml:"definition,omitempty"`

	releasePresent    bool
	definitionPresent bool
}

// ConnectorReleaseSpecPatch contains a desired release-state transition.
type ConnectorReleaseSpecPatch struct {
	DesiredState *ConnectorReleaseState `json:"desiredState,omitempty" yaml:"desiredState,omitempty"`

	desiredStatePresent bool
}

func NewConnector() *Connector {
	return &Connector{TypeMeta: meta.NewTypeMeta(ConnectorKind)}
}

// NewConnectorPatch returns an empty Connector update envelope.
func NewConnectorPatch() *ConnectorPatch {
	return &ConnectorPatch{
		TypeMeta: meta.NewTypeMeta(ConnectorKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &ConnectorSpecPatch{},
	}
}

// ApplyAPICreateDefaults returns a copy with server-assigned identity and
// generation. API-created connectors default to draft so publishing remains an
// explicit release decision; configuration retains its existing primary
// default during reconciliation.
func (c *Connector) ApplyAPICreateDefaults(id apid.ID) *Connector {
	clone := c.Clone()
	clone.Metadata.ID = id.String()
	clone.Metadata.Generation = 1
	if clone.Metadata.Name == "" {
		clone.Metadata.Name = common.ResourceName(id.String())
	}
	if clone.Spec.Release.DesiredState == "" {
		clone.Spec.Release.DesiredState = ConnectorReleaseStateDraft
	}
	return clone
}

func (c *Connector) Clone() *Connector {
	if c == nil {
		return nil
	}

	clone := *c
	clone.Metadata = meta.CloneObjectMeta(c.Metadata)
	clone.Spec.Definition = *c.Spec.Definition.Clone()
	if c.Status != nil {
		status := *c.Status
		clone.Status = &status
	}
	return &clone
}

// Validate applies configuration-file resource rules. API handlers use
// ValidateFor with the lifecycle mode appropriate to the request.
func (c *Connector) Validate(vc *common.ValidationContext) error {
	return c.ValidateFor(meta.ValidationModeConfig, vc)
}

func (c *Connector) ValidateFor(
	mode meta.ValidationMode,
	vc *common.ValidationContext,
) error {
	if c == nil {
		return fmt.Errorf("connector is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	requireStoredIdentity := mode == meta.ValidationModePersistence ||
		mode == meta.ValidationModeResponse
	requireNamespace := mode != meta.ValidationModeConfig

	var result *multierror.Error
	if err := meta.ValidateResource(
		c.TypeMeta,
		c.Metadata,
		meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       ConnectorKind,
			RequireID:          requireStoredIdentity,
			RequireName:        requireStoredIdentity,
			RequireNamespace:   requireNamespace,
			IDValidator:        ValidateID,
			NamespaceValidator: nschema.ValidatePath,
		},
	); err != nil {
		result = multierror.Append(result, err)
	}
	if requireStoredIdentity && c.Metadata.Generation == 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "is required"))
	}

	switch c.Spec.Release.DesiredState {
	case "", ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary:
	default:
		result = multierror.Append(result, vc.NewErrorfForField("spec.release.desiredState", "must be either %q or %q", ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary))
	}

	if err := c.Spec.Definition.Validate(vc.
		PushField("spec").
		PushField("definition"),
	); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(c.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Status != nil {
		switch c.Status.Release.State {
		case ConnectorReleaseStateDraft,
			ConnectorReleaseStatePrimary,
			ConnectorReleaseStateActive,
			ConnectorReleaseStateArchived:
		default:
			result = multierror.Append(result, vc.NewErrorfForField("status.release.state", "is not a recognized connector release state"))
		}
	}
	if requireStoredIdentity && c.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}
	if requireStoredIdentity && c.Spec.Release.DesiredState == "" {
		result = multierror.Append(result, vc.NewErrorForField("spec.release.desiredState", "is required"))
	}
	if requireStoredIdentity && c.Status != nil {
		observed := c.Status.Release.State
		switch c.Spec.Release.DesiredState {
		case ConnectorReleaseStateDraft:
			if observed != ConnectorReleaseStateDraft {
				result = multierror.Append(result, vc.NewErrorForField("status.release.state", "must be draft when desiredState is draft"))
			}
		case ConnectorReleaseStatePrimary:
			if observed != ConnectorReleaseStatePrimary &&
				observed != ConnectorReleaseStateActive &&
				observed != ConnectorReleaseStateArchived {
				result = multierror.Append(result, vc.NewErrorForField("status.release.state", "must be primary, active, or archived when desiredState is primary"))
			}
		}
	}

	return result.ErrorOrNil()
}

// ValidateID validates the stable logical connector identifier.
func ValidateID(value string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != apid.PrefixConnector {
		return fmt.Errorf("must be a connector id")
	}
	return nil
}

// DesiredReleaseStateForObserved maps the database's observed lifecycle state
// back to declarative intent. Draft generations remain draft; every published
// generation retains primary intent even after it becomes active or archived.
func DesiredReleaseStateForObserved(state ConnectorReleaseState) ConnectorReleaseState {
	if state == ConnectorReleaseStateDraft {
		return ConnectorReleaseStateDraft
	}
	return ConnectorReleaseStatePrimary
}

// ValidateFor validates a partial Connector at the API update boundary.
func (c *ConnectorPatch) ValidateFor(
	mode meta.ValidationMode,
	vc *common.ValidationContext,
) error {
	if c == nil {
		return fmt.Errorf("connector patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(c.TypeMeta, meta.APIVersionV1Alpha1, ConnectorKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else if err := meta.ValidateObjectMetaPatch(*c.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		IDValidator:        ValidateID,
		NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	} else if err := c.Spec.validate(vc.PushField("spec")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(c.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func (c *ConnectorSpecPatch) validate(vc *common.ValidationContext) error {
	var result *multierror.Error
	if c.releasePresent && c.Release == nil {
		result = multierror.Append(result, vc.NewErrorForField("release", "must not be null"))
	} else if c.Release != nil {
		if c.Release.desiredStatePresent && c.Release.DesiredState == nil {
			result = multierror.Append(result, vc.NewErrorForField("release.desiredState", "must not be null"))
		} else if c.Release.DesiredState != nil {
			switch *c.Release.DesiredState {
			case ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary:
			default:
				result = multierror.Append(result, vc.NewErrorfForField("release.desiredState", "must be either %q or %q", ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary))
			}
		}
	}
	if c.definitionPresent && c.Definition == nil {
		result = multierror.Append(result, vc.NewErrorForField("definition", "must not be null"))
	} else if c.Definition != nil {
		if err := c.Definition.Validate(vc.PushField("definition")); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result.ErrorOrNil()
}

// HasRelease reports whether the patch contains a release object, including an
// explicit null value.
func (c *ConnectorSpecPatch) HasRelease() bool {
	return c != nil && (c.releasePresent || c.Release != nil)
}

// HasDefinition reports whether the patch contains a definition, including an
// explicit null value.
func (c *ConnectorSpecPatch) HasDefinition() bool {
	return c != nil && (c.definitionPresent || c.Definition != nil)
}

// HasDesiredState reports whether desiredState was provided, including null.
func (c *ConnectorReleaseSpecPatch) HasDesiredState() bool {
	return c != nil && (c.desiredStatePresent || c.DesiredState != nil)
}

// ApplyTo applies this patch to a copy of current and enforces immutable
// connector identity and generation fields.
func (c *ConnectorPatch) ApplyTo(
	current *Connector,
	vc *common.ValidationContext,
) (*Connector, error) {
	if current == nil {
		return nil, fmt.Errorf("current connector is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}
	if err := c.ValidateFor(meta.ValidationModeUpdate, vc); err != nil {
		return nil, err
	}

	updated := current.Clone()
	updated.Metadata = meta.ApplyObjectMetaPatch(updated.Metadata, *c.Metadata)
	if c.Spec.Release != nil && c.Spec.Release.DesiredState != nil {
		updated.Spec.Release.DesiredState = *c.Spec.Release.DesiredState
	}
	if c.Spec.Definition != nil {
		updated.Spec.Definition = *c.Spec.Definition.Clone()
	}
	if err := ValidateUpdate(current, updated, vc); err != nil {
		return nil, err
	}
	return updated, nil
}

// ValidateUpdate rejects changes to stable connector identity. Names, labels,
// annotations, provider definitions, and desired release state are mutable;
// namespace and generation are not.
func ValidateUpdate(before, after *Connector, vc *common.ValidationContext) error {
	if before == nil || after == nil {
		return fmt.Errorf("before and after connectors are required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMetaUpdate(before.TypeMeta, after.TypeMeta, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateMetadataUpdate(before.Metadata, after.Metadata, meta.UpdateOptions{ImmutableNamespace: true}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (c *Connector) HasId() bool { return c != nil && c.Metadata.ID != "" }

func (c *Connector) GetId() apid.ID {
	if !c.HasId() {
		return apid.Nil
	}
	id, err := apid.Parse(c.Metadata.ID)
	if err != nil {
		return apid.Nil
	}
	return id
}

func (c *Connector) SetId(id apid.ID) {
	if c != nil {
		c.Metadata.ID = id.String()
	}
}

func (c *Connector) HasName() bool {
	return c != nil && c.Metadata.Name != ""
}

func (c *Connector) HasGeneration() bool {
	return c != nil && c.Metadata.Generation > 0
}

func (c *Connector) HasState() bool {
	return c != nil && c.Spec.Release.DesiredState != ""
}

func (c *Connector) HasNamespace() bool {
	return c != nil && c.Metadata.Namespace != ""
}

func (c *Connector) IsDraft() bool {
	return c != nil && c.Spec.Release.DesiredState == ConnectorReleaseStateDraft
}
func (c *Connector) GetNamespace() string {
	if c == nil || c.Metadata.Namespace == "" {
		return nschema.Root
	}
	return c.Metadata.Namespace
}

func (c *Connector) DefinitionHash() string {
	if c == nil {
		return ""
	}
	return c.Spec.Definition.Hash()
}
