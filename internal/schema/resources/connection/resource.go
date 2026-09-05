package connection

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ConnectionState is the server-observed lifecycle state of a connection.
type ConnectionState string

const (
	ConnectionStateSetup         ConnectionState = "setup"
	ConnectionStateConfigured    ConnectionState = "configured"
	ConnectionStateDisabled      ConnectionState = "disabled"
	ConnectionStateDisconnecting ConnectionState = "disconnecting"
	ConnectionStateDisconnected  ConnectionState = "disconnected"
)

// ConnectionHealthState is the server-observed operational health of a
// connection. It is independent of lifecycle state: a configured connection
// may be unhealthy and require reauthentication.
type ConnectionHealthState string

const (
	ConnectionHealthStateHealthy   ConnectionHealthState = "healthy"
	ConnectionHealthStateUnhealthy ConnectionHealthState = "unhealthy"
)

// Connection is the canonical Kubernetes-style representation of one
// configured connector instance. ConnectorRef is generation-specific because
// credentials and setup configuration are interpreted by that exact connector
// definition. ActorRef records the actor that initiated the connection; it is
// informational and does not replace namespace-based authorization.
type Connection struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta   `json:"metadata" yaml:"metadata"`
	Spec          ConnectionSpec    `json:"spec" yaml:"spec"`
	Status        *ConnectionStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// ConnectionSpec contains the stable references and connector-defined desired
// configuration for a connection. Clients change configuration through typed
// setup actions; API responses always redact every configuration value.
type ConnectionSpec struct {
	ConnectorRef  meta.ObjectReference  `json:"connectorRef" yaml:"connectorRef"`
	ActorRef      *meta.ObjectReference `json:"actorRef,omitempty" yaml:"actorRef,omitempty"`
	Configuration map[string]any        `json:"configuration,omitempty" yaml:"configuration,omitempty" apiredact:"secret"`
}

// ConnectionStatus contains server-observed lifecycle, health, setup, and
// encrypted-configuration state.
type ConnectionStatus struct {
	Lifecycle               ConnectionLifecycleStatus `json:"lifecycle" yaml:"lifecycle"`
	Health                  ConnectionHealthStatus    `json:"health" yaml:"health"`
	Setup                   *ConnectionSetupStatus    `json:"setup,omitempty" yaml:"setup,omitempty"`
	ConfigurationConfigured bool                      `json:"configurationConfigured" yaml:"configurationConfigured"`
}

type ConnectionLifecycleStatus struct {
	State ConnectionState `json:"state" yaml:"state"`
}

// ConnectionHealthStatus is the aggregate probe/credential health
// observation. Individual probe histories remain operational data rather than
// part of the durable Connection resource.
type ConnectionHealthStatus struct {
	State ConnectionHealthState `json:"state" yaml:"state"`
}

// ConnectionSetupStatus records the current setup step and any terminal setup
// error without exposing submitted values or credentials.
type ConnectionSetupStatus struct {
	StepID string  `json:"stepId,omitempty" yaml:"stepId,omitempty"`
	Error  *string `json:"error,omitempty" yaml:"error,omitempty"`
}

// ConnectionPatch is a metadata-only update. Connection bindings and all
// status fields are immutable through CRUD; connector changes occur through
// the version-migration action and setup state changes through setup actions.
type ConnectionPatch struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      *meta.ObjectMetaPatch `json:"metadata" yaml:"metadata"`
	Spec          *ConnectionSpecPatch  `json:"spec" yaml:"spec"`
	Status        *ConnectionStatus     `json:"status,omitempty" yaml:"status,omitempty"`
}

// ConnectionSpecPatch is intentionally empty. Keeping the required object in
// the wire contract makes immutable desired state explicit and leaves room for
// future mutable policy without weakening strict decoding.
type ConnectionSpecPatch struct{}

func NewConnection() *Connection {
	return &Connection{TypeMeta: meta.NewTypeMeta(ConnectionKind)}
}

func NewConnectionPatch() *ConnectionPatch {
	return &ConnectionPatch{
		TypeMeta: meta.NewTypeMeta(ConnectionKind),
		Metadata: &meta.ObjectMetaPatch{},
		Spec:     &ConnectionSpecPatch{},
	}
}

func NewConnectionReference(id apid.ID) meta.ObjectReference {
	return meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       ConnectionKind,
		ID:         id.String(),
	}
}

func NewActorReference(id apid.ID) *meta.ObjectReference {
	if id.IsNil() {
		return nil
	}
	return &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       actorschema.ActorKind,
		ID:         id.String(),
	}
}

// RedactConfiguration returns a deep-copied configuration with every value
// masked. It deliberately ignores secret-replay authorization: connection
// setup values and credentials are write-only, even for callers that may
// replay secrets from other resource types.
func RedactConfiguration(configuration map[string]any) (map[string]any, error) {
	if configuration == nil {
		return nil, nil
	}

	value := struct {
		Configuration map[string]any `json:"configuration" apiredact:"secret"`
	}{Configuration: configuration}
	data, _, err := apserde.MarshalJSONForAPI(context.Background(), value)
	if err != nil {
		return nil, fmt.Errorf("redact connection configuration: %w", err)
	}

	var redacted struct {
		Configuration map[string]any `json:"configuration"`
	}
	if err := json.Unmarshal(data, &redacted); err != nil {
		return nil, fmt.Errorf("decode redacted connection configuration: %w", err)
	}
	return redacted.Configuration, nil
}

func ValidateID(value string) error {
	id, err := apid.Parse(value)
	if err != nil {
		return err
	}
	if id.Prefix() != apid.PrefixConnection {
		return fmt.Errorf("must be a connection id")
	}
	return nil
}

func IsValidConnectionState(state ConnectionState) bool {
	switch state {
	case ConnectionStateSetup,
		ConnectionStateConfigured,
		ConnectionStateDisabled,
		ConnectionStateDisconnecting,
		ConnectionStateDisconnected:
		return true
	default:
		return false
	}
}

func IsValidConnectionHealthState(state ConnectionHealthState) bool {
	switch state {
	case ConnectionHealthStateHealthy, ConnectionHealthStateUnhealthy:
		return true
	default:
		return false
	}
}

func (c *Connection) Clone() *Connection {
	if c == nil {
		return nil
	}
	clone := *c
	clone.Metadata = meta.CloneObjectMeta(c.Metadata)
	clone.Spec.Configuration = cloneConfiguration(c.Spec.Configuration)
	if c.Spec.ActorRef != nil {
		actorRef := *c.Spec.ActorRef
		clone.Spec.ActorRef = &actorRef
	}
	if c.Status != nil {
		status := *c.Status
		if c.Status.Setup != nil {
			setup := *c.Status.Setup
			if setup.Error != nil {
				setupError := *setup.Error
				setup.Error = &setupError
			}
			status.Setup = &setup
		}
		clone.Status = &status
	}
	return &clone
}

func cloneConfiguration(configuration map[string]any) map[string]any {
	if configuration == nil {
		return nil
	}
	cloned := make(map[string]any, len(configuration))
	for key, value := range configuration {
		cloned[key] = cloneConfigurationValue(value)
	}
	return cloned
}

func cloneConfigurationValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneConfiguration(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneConfigurationValue(item)
		}
		return cloned
	default:
		return value
	}
}

func (c *Connection) Validate(vc *common.ValidationContext) error {
	return c.ValidateFor(meta.ValidationModeConfig, vc)
}

func (c *Connection) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if c == nil {
		return fmt.Errorf("connection is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	requireStoredIdentity := mode == meta.ValidationModePersistence || mode == meta.ValidationModeResponse
	var result *multierror.Error
	if err := meta.ValidateResource(c.TypeMeta, c.Metadata, meta.ValidationOptions{
		Mode:               mode,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       ConnectionKind,
		RequireID:          requireStoredIdentity,
		RequireName:        requireStoredIdentity,
		RequireNamespace:   true,
		IDValidator:        ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to connections"))
	}

	if err := validateConnectorReference(c.Spec.ConnectorRef, requireStoredIdentity, vc.PushField("spec").PushField("connectorRef")); err != nil {
		result = multierror.Append(result, err)
	}
	if c.Spec.ActorRef != nil {
		if err := meta.ValidateObjectReferenceWithOptions(*c.Spec.ActorRef, meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       actorschema.ActorKind,
			IDValidator:        actorschema.ValidateID,
			NamespaceValidator: namespaceschema.ValidatePath,
		}, vc.PushField("spec").PushField("actorRef")); err != nil {
			result = multierror.Append(result, err)
		}
		if c.Spec.ActorRef.Generation != 0 {
			result = multierror.Append(result, vc.NewErrorForField("spec.actorRef.generation", "does not apply to actors"))
		}
	}

	if err := meta.ValidateStatus(c.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if requireStoredIdentity && c.Status == nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is required"))
	}
	if c.Status != nil {
		if !IsValidConnectionState(c.Status.Lifecycle.State) {
			result = multierror.Append(result, vc.NewErrorForField("status.lifecycle.state", "is not a recognized connection lifecycle state"))
		}
		if !IsValidConnectionHealthState(c.Status.Health.State) {
			result = multierror.Append(result, vc.NewErrorForField("status.health.state", "is not a recognized connection health state"))
		}
		if c.Status.Setup != nil && c.Status.Setup.StepID == "" && c.Status.Setup.Error == nil {
			result = multierror.Append(result, vc.NewErrorForField("status.setup", "must contain stepId or error"))
		}
	}

	return result.ErrorOrNil()
}

func validateConnectorReference(ref meta.ObjectReference, requireGeneration bool, vc *common.ValidationContext) error {
	var result *multierror.Error
	if err := meta.ValidateObjectReferenceWithOptions(ref, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       connectorschema.ConnectorKind,
		IDValidator:        connectorschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if requireGeneration && ref.Generation == 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "is required for a stored connection binding"))
	}
	return result.ErrorOrNil()
}

func (p *ConnectionPatch) ValidateFor(mode meta.ValidationMode, vc *common.ValidationContext) error {
	if p == nil {
		return fmt.Errorf("connection patch is required")
	}
	if vc == nil {
		vc = &common.ValidationContext{Path: "$"}
	}

	var result *multierror.Error
	if err := meta.ValidateTypeMeta(p.TypeMeta, meta.APIVersionV1Alpha1, ConnectionKind, vc); err != nil {
		result = multierror.Append(result, err)
	}
	if p.Metadata == nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata", "is required and must not be null"))
	} else {
		if err := meta.ValidateObjectMetaPatch(*p.Metadata, meta.ValidationOptions{
			Mode:               mode,
			Path:               vc,
			IDValidator:        ValidateID,
			NamespaceValidator: namespaceschema.ValidatePath,
		}); err != nil {
			result = multierror.Append(result, err)
		}
		if p.Metadata.Generation != nil {
			result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "does not apply to connections"))
		}
	}
	if p.Spec == nil {
		result = multierror.Append(result, vc.NewErrorForField("spec", "is required and must not be null"))
	}
	if err := meta.ValidateStatus(p.Status, mode, vc); err != nil {
		result = multierror.Append(result, err)
	}
	return result.ErrorOrNil()
}

func (c *Connection) ApplyUpdate(patch *ConnectionPatch) (*Connection, error) {
	if err := patch.ValidateFor(meta.ValidationModeUpdate, nil); err != nil {
		return nil, err
	}
	result := c.Clone()
	result.Metadata = meta.ApplyObjectMetaPatch(result.Metadata, *patch.Metadata)
	if err := meta.ValidateTypeMetaUpdate(c.TypeMeta, result.TypeMeta, nil); err != nil {
		return nil, err
	}
	if err := meta.ValidateMetadataUpdate(c.Metadata, result.Metadata, meta.UpdateOptions{ImmutableNamespace: true}, nil); err != nil {
		return nil, err
	}
	return result, result.ValidateFor(meta.ValidationModeResponse, nil)
}
