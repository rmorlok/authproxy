package api

import (
	"fmt"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	ConnectionDisconnectActionKind       meta.Kind = "ConnectionDisconnect"
	ConnectionVersionMigrationActionKind meta.Kind = "ConnectionVersionMigration"
	ConnectionForceStateActionKind       meta.Kind = "ConnectionForceState"
)

type ListConnectionResponseJson struct {
	apiv1alpha1.ResourceList[connectionschema.Connection] `json:",inline" yaml:",inline"`
}

func NewListConnectionResponseJson(
	items []connectionschema.Connection,
	continueToken string,
) ListConnectionResponseJson {
	return ListConnectionResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			connectionschema.ConnectionKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

type ConnectionDisconnectSpec struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
}

type ConnectionDisconnectStatus struct {
	TaskID     string                      `json:"taskId" yaml:"taskId"`
	Connection connectionschema.Connection `json:"connection" yaml:"connection"`
}

type ConnectionDisconnectAction struct {
	apiv1alpha1.Action[ConnectionDisconnectSpec, ConnectionDisconnectStatus] `json:",inline" yaml:",inline"`
}

func (a *ConnectionDisconnectAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return a.validateFields(false)
}

func (a *ConnectionDisconnectAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	return a.validateFields(true)
}

func (a *ConnectionDisconnectAction) validateFields(requireStatus bool) error {
	if err := validateConnectionActionTarget(a.Metadata.Target); err != nil {
		return err
	}
	if a.Spec.TimeoutSeconds != nil && *a.Spec.TimeoutSeconds <= 0 {
		return fmt.Errorf("$.spec.timeoutSeconds: must be greater than zero")
	}
	if !requireStatus {
		return nil
	}
	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if a.Status.TaskID == "" {
		return fmt.Errorf("$.status.taskId: is required")
	}
	if err := a.Status.Connection.ValidateFor(meta.ValidationModeResponse, nil); err != nil {
		return fmt.Errorf("$.status.connection: %w", err)
	}
	return nil
}

func NewConnectionDisconnectResponse(
	target meta.ObjectReference,
	spec ConnectionDisconnectSpec,
	status ConnectionDisconnectStatus,
) ConnectionDisconnectAction {
	return ConnectionDisconnectAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionDisconnectActionKind,
		target,
		spec,
		status,
	)}
}

// ConnectionVersionMigrationSpec identifies the exact target connector
// generation. The action target remains the Connection being migrated.
type ConnectionVersionMigrationSpec struct {
	ConnectorRef   meta.ObjectReference `json:"connectorRef" yaml:"connectorRef"`
	TimeoutSeconds *int64               `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
}

type ConnectionVersionMigrationStatus struct {
	TaskID             string               `json:"taskId" yaml:"taskId"`
	SourceConnectorRef meta.ObjectReference `json:"sourceConnectorRef" yaml:"sourceConnectorRef"`
	TargetConnectorRef meta.ObjectReference `json:"targetConnectorRef" yaml:"targetConnectorRef"`
}

type ConnectionVersionMigrationAction struct {
	apiv1alpha1.Action[ConnectionVersionMigrationSpec, ConnectionVersionMigrationStatus] `json:",inline" yaml:",inline"`
}

func (a *ConnectionVersionMigrationAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return a.validateFields(false)
}

func (a *ConnectionVersionMigrationAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	return a.validateFields(true)
}

func (a *ConnectionVersionMigrationAction) validateFields(requireStatus bool) error {
	if err := validateConnectionActionTarget(a.Metadata.Target); err != nil {
		return err
	}
	if err := validateVersionedConnectorReference(a.Spec.ConnectorRef, "$.spec.connectorRef"); err != nil {
		return err
	}
	if a.Spec.TimeoutSeconds != nil && *a.Spec.TimeoutSeconds <= 0 {
		return fmt.Errorf("$.spec.timeoutSeconds: must be greater than zero")
	}
	if !requireStatus {
		return nil
	}
	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if a.Status.TaskID == "" {
		return fmt.Errorf("$.status.taskId: is required")
	}
	if err := validateVersionedConnectorReference(a.Status.SourceConnectorRef, "$.status.sourceConnectorRef"); err != nil {
		return err
	}
	if err := validateVersionedConnectorReference(a.Status.TargetConnectorRef, "$.status.targetConnectorRef"); err != nil {
		return err
	}
	return nil
}

func validateVersionedConnectorReference(reference meta.ObjectReference, path string) error {
	vc := &common.ValidationContext{Path: path}
	if err := meta.ValidateObjectReferenceWithOptions(reference, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       connectorschema.ConnectorKind,
		IDValidator:        connectorschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc); err != nil {
		return err
	}
	if reference.Generation == 0 {
		return vc.NewErrorForField("generation", "is required")
	}
	return nil
}

func NewConnectionVersionMigrationResponse(
	target meta.ObjectReference,
	spec ConnectionVersionMigrationSpec,
	status ConnectionVersionMigrationStatus,
) ConnectionVersionMigrationAction {
	return ConnectionVersionMigrationAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionVersionMigrationActionKind,
		target,
		spec,
		status,
	)}
}

type ConnectionForceStateSpec struct {
	State connectionschema.ConnectionState `json:"state" yaml:"state"`
}

type ConnectionForceStateStatus struct {
	Connection connectionschema.Connection `json:"connection" yaml:"connection"`
}

type ConnectionForceStateAction struct {
	apiv1alpha1.Action[ConnectionForceStateSpec, ConnectionForceStateStatus] `json:",inline" yaml:",inline"`
}

func (a *ConnectionForceStateAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return a.validateFields(false)
}

func (a *ConnectionForceStateAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	return a.validateFields(true)
}

func (a *ConnectionForceStateAction) validateFields(requireStatus bool) error {
	if err := validateConnectionActionTarget(a.Metadata.Target); err != nil {
		return err
	}
	if !connectionschema.IsValidConnectionState(a.Spec.State) {
		return fmt.Errorf("$.spec.state: is not a recognized connection lifecycle state")
	}
	if !requireStatus {
		return nil
	}
	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if err := a.Status.Connection.ValidateFor(meta.ValidationModeResponse, nil); err != nil {
		return fmt.Errorf("$.status.connection: %w", err)
	}
	return nil
}

func NewConnectionForceStateResponse(
	target meta.ObjectReference,
	spec ConnectionForceStateSpec,
	connection connectionschema.Connection,
) ConnectionForceStateAction {
	return ConnectionForceStateAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionForceStateActionKind,
		target,
		spec,
		ConnectionForceStateStatus{Connection: connection},
	)}
}

// ProxyResponseJson is the unwrapped response from a proxied request.
type ProxyResponseJson struct {
	StatusCode int               `json:"statusCode" yaml:"statusCode" example:"200"`
	Headers    map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	BodyRaw    []byte            `json:"bodyRaw,omitempty" yaml:"bodyRaw,omitempty"`
	BodyJson   interface{}       `json:"bodyJson,omitempty" yaml:"bodyJson,omitempty"`
}
