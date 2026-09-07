package api

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	ConnectorDisconnectAllActionKind meta.Kind = "ConnectorDisconnectAll"
	ConnectorArchiveActionKind       meta.Kind = "ConnectorArchive"
	ConnectorForceStateActionKind    meta.Kind = "ConnectorForceState"
)

type ListConnectorsResponseJson struct {
	apiv1alpha1.ResourceList[cschema.Connector] `json:",inline" yaml:",inline"`
}

func NewListConnectorsResponseJson(
	items []cschema.Connector,
	continueToken string,
) ListConnectorsResponseJson {
	return ListConnectorsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			cschema.ConnectorKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

type ListConnectorVersionsResponseJson struct {
	apiv1alpha1.ResourceList[cschema.Connector] `json:",inline" yaml:",inline"`
}

func NewListConnectorVersionsResponseJson(
	items []cschema.Connector,
	continueToken string,
) ListConnectorVersionsResponseJson {
	return ListConnectorVersionsResponseJson{
		ResourceList: apiv1alpha1.NewResourceList(
			cschema.ConnectorKind,
			items,
			apiv1alpha1.ListMeta{Continue: continueToken},
		),
	}
}

// ConnectorLifecycleSpec contains options for connector-level lifecycle
// operations that run asynchronously.
type ConnectorLifecycleSpec struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty" example:"600"`
}

// ConnectorLifecycleStatus identifies the asynchronous task created for a
// lifecycle operation. The connector itself is identified by metadata.target.
type ConnectorLifecycleStatus struct {
	TaskID string `json:"taskId" yaml:"taskId"`
}

// ConnectorLifecycleAction is shared by disconnect-all and archive. Its kind
// distinguishes the operation and metadata.target identifies the logical
// connector across all generations.
type ConnectorLifecycleAction struct {
	apiv1alpha1.Action[ConnectorLifecycleSpec, ConnectorLifecycleStatus] `json:",inline" yaml:",inline"`
}

func NewConnectorLifecycleRequest(
	kind meta.Kind,
	target meta.ObjectReference,
	spec ConnectorLifecycleSpec,
) ConnectorLifecycleAction {
	request := apiv1alpha1.NewActionRequest(kind, target, spec)
	return ConnectorLifecycleAction{Action: apiv1alpha1.Action[ConnectorLifecycleSpec, ConnectorLifecycleStatus]{
		TypeMeta: request.TypeMeta,
		Metadata: request.Metadata,
		Spec:     request.Spec,
	}}
}

func (a *ConnectorLifecycleAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return a.validateFields(false)
}

func (a *ConnectorLifecycleAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	return a.validateFields(true)
}

func (a *ConnectorLifecycleAction) validateFields(requireStatus bool) error {
	if err := validateConnectorActionTarget(a.Metadata.Target, false); err != nil {
		return err
	}
	if a.Spec.TimeoutSeconds != nil && *a.Spec.TimeoutSeconds <= 0 {
		return fmt.Errorf("$.spec.timeoutSeconds: must be greater than zero")
	}
	if requireStatus && a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}
	if requireStatus && a.Status.TaskID == "" {
		return fmt.Errorf("$.status.taskId: is required")
	}
	return nil
}

func NewConnectorLifecycleResponse(
	kind meta.Kind,
	target meta.ObjectReference,
	spec ConnectorLifecycleSpec,
	taskID string,
) ConnectorLifecycleAction {
	return ConnectorLifecycleAction{Action: apiv1alpha1.NewActionResponse(
		kind,
		target,
		spec,
		ConnectorLifecycleStatus{TaskID: taskID},
	)}
}

// ConnectorForceStateSpec is the desired observed state for one exact
// connector generation.
type ConnectorForceStateSpec struct {
	State cschema.ConnectorReleaseState `json:"state" yaml:"state" example:"primary"`
}

type ConnectorForceStateAction struct {
	apiv1alpha1.Action[ConnectorForceStateSpec, struct{}] `json:",inline" yaml:",inline"`
}

func NewConnectorForceStateRequest(
	target meta.ObjectReference,
	state cschema.ConnectorReleaseState,
) ConnectorForceStateAction {
	request := apiv1alpha1.NewActionRequest(
		ConnectorForceStateActionKind,
		target,
		ConnectorForceStateSpec{State: state},
	)
	return ConnectorForceStateAction{Action: request}
}

func (a *ConnectorForceStateAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	if err := validateConnectorActionTarget(a.Metadata.Target, true); err != nil {
		return err
	}
	switch a.Spec.State {
	case cschema.ConnectorReleaseStateDraft,
		cschema.ConnectorReleaseStatePrimary,
		cschema.ConnectorReleaseStateActive,
		cschema.ConnectorReleaseStateArchived:
		return nil
	default:
		return fmt.Errorf("$.spec.state: is not a recognized connector release state")
	}
}

func validateConnectorActionTarget(target meta.ObjectReference, requireGeneration bool) error {
	vc := &common.ValidationContext{Path: "$.metadata.target"}
	var result *multierror.Error
	if err := meta.ValidateObjectReferenceWithOptions(
		target,
		meta.ObjectReferenceValidationOptions{
			ExpectedAPIVersion: meta.APIVersionV1Alpha1,
			ExpectedKind:       cschema.ConnectorKind,
			IDValidator:        cschema.ValidateID,
			NamespaceValidator: nschema.ValidatePath,
		},
		vc,
	); err != nil {
		result = multierror.Append(result, err)
	}
	if target.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("id", "is required for connector action targets"))
	}
	if target.Name != "" || target.Namespace != "" {
		result = multierror.Append(result, vc.NewError("connector action targets support id only"))
	}
	if requireGeneration && target.Generation == 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "is required"))
	}
	if !requireGeneration && target.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "does not apply to logical connector actions"))
	}
	return result.ErrorOrNil()
}
