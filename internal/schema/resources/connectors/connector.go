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

func NewConnector() *Connector {
	return &Connector{TypeMeta: meta.NewTypeMeta(ConnectorKind)}
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

func (c *Connector) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if c == nil {
		return fmt.Errorf("connector is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateResource(c.TypeMeta, c.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       ConnectorKind,
		IDValidator: func(value string) error {
			id, err := apid.Parse(value)
			if err != nil {
				return err
			}
			if id.Prefix() != apid.PrefixConnector {
				return fmt.Errorf("must be a connector id")
			}
			return nil
		},
	}); err != nil {
		result = multierror.Append(result, err)
	}

	if c.Metadata.Namespace != "" {
		if err := nschema.ValidatePath(c.Metadata.Namespace); err != nil {
			result = multierror.Append(result, err)
		}
	}

	switch c.Spec.Release.DesiredState {
	case "", ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary:
	default:
		result = multierror.Append(result, vc.NewErrorfForField("spec.release.desiredState", "must be either %q or %q", ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary))
	}

	if err := c.Spec.Definition.Validate(vc.PushField("spec").PushField("definition")); err != nil {
		result = multierror.Append(result, err)
	}
	if err := meta.ValidateStatus(c.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Status != nil {
		switch c.Status.Release.State {
		case ConnectorReleaseStateDraft, ConnectorReleaseStatePrimary, ConnectorReleaseStateActive, ConnectorReleaseStateArchived:
		default:
			result = multierror.Append(result, vc.NewErrorfForField("status.release.state", "is not a recognized connector release state"))
		}
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

func (c *Connector) HasName() bool    { return c != nil && c.Metadata.Name != "" }
func (c *Connector) HasVersion() bool { return c != nil && c.Metadata.Generation > 0 }
func (c *Connector) HasState() bool {
	return c != nil && c.Spec.Release.DesiredState != ""
}
func (c *Connector) HasNamespace() bool { return c != nil && c.Metadata.Namespace != "" }
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
