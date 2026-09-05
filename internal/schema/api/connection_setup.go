package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apserde"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const (
	ConnectionInitiateActionKind    meta.Kind = "ConnectionInitiate"
	ConnectionSetupActionKind       meta.Kind = "ConnectionSetup"
	ConnectionSetupSubmitActionKind meta.Kind = "ConnectionSetupSubmit"
	ConnectionSetupAbortActionKind  meta.Kind = "ConnectionSetupAbort"
	ConnectionReconfigureActionKind meta.Kind = "ConnectionReconfigure"
	ConnectionSetupCancelActionKind meta.Kind = "ConnectionSetupCancel"
	ConnectionSetupRetryActionKind  meta.Kind = "ConnectionSetupRetry"
	ConnectionReauthActionKind      meta.Kind = "ConnectionReauthenticate"
)

// ConnectionInitiateSpec contains the desired metadata and callback used to
// create a connection. The action target is the Connector reference; an
// omitted target generation selects its primary generation.
type ConnectionInitiateSpec struct {
	IntoNamespace string               `json:"intoNamespace,omitempty" yaml:"intoNamespace,omitempty"`
	Name          *common.ResourceName `json:"name,omitempty" yaml:"name,omitempty"`
	Labels        map[string]string    `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations   map[string]string    `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	ReturnToURL   string               `json:"returnToUrl" yaml:"returnToUrl"`
}

// ConnectionInitiateAction starts setup for a new Connection.
type ConnectionInitiateAction struct {
	apiv1alpha1.Action[ConnectionInitiateSpec, struct{}] `json:",inline" yaml:",inline"`
}

func (a *ConnectionInitiateAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	vc := &common.ValidationContext{Path: "$"}
	var result *multierror.Error
	if err := meta.ValidateObjectReferenceWithOptions(a.Metadata.Target, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       connectorschema.ConnectorKind,
		IDValidator:        connectorschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc.PushField("metadata").PushField("target")); err != nil {
		result = multierror.Append(result, err)
	}
	if a.Spec.IntoNamespace != "" {
		if err := namespaceschema.ValidatePath(a.Spec.IntoNamespace); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("spec.intoNamespace", "%v", err))
		}
	}
	if a.Spec.Name != nil {
		if err := a.Spec.Name.Validate(); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("spec.name", "%v", err))
		}
	}
	if err := meta.ValidateUserLabels(a.Spec.Labels); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("spec.labels", "%v", err))
	}
	if err := meta.ValidateAnnotations(a.Spec.Annotations); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("spec.annotations", "%v", err))
	}
	if a.Spec.ReturnToURL == "" {
		result = multierror.Append(result, vc.NewErrorForField("spec.returnToUrl", "is required"))
	}
	return result.ErrorOrNil()
}

// ConnectionSetupResponseType identifies the next setup UI/protocol step.
type ConnectionSetupResponseType string

const (
	ConnectionSetupResponseTypeRedirect  ConnectionSetupResponseType = "redirect"
	ConnectionSetupResponseTypeForm      ConnectionSetupResponseType = "form"
	ConnectionSetupResponseTypeComplete  ConnectionSetupResponseType = "complete"
	ConnectionSetupResponseTypeVerifying ConnectionSetupResponseType = "verifying"
	ConnectionSetupResponseTypeError     ConnectionSetupResponseType = "error"
)

// ConnectionSetupActionStatus is the observed result of a setup operation.
// Data may contain previously submitted setup values and is therefore always
// returned in irreversibly redacted form.
type ConnectionSetupActionStatus struct {
	Type            ConnectionSetupResponseType `json:"type" yaml:"type"`
	RedirectURL     string                      `json:"redirectUrl,omitempty" yaml:"redirectUrl,omitempty"`
	StepID          string                      `json:"stepId,omitempty" yaml:"stepId,omitempty"`
	StepTitle       string                      `json:"stepTitle,omitempty" yaml:"stepTitle,omitempty"`
	StepDescription string                      `json:"stepDescription,omitempty" yaml:"stepDescription,omitempty"`
	JSONSchema      json.RawMessage             `json:"jsonSchema,omitempty" yaml:"jsonSchema,omitempty"`
	UISchema        json.RawMessage             `json:"uiSchema,omitempty" yaml:"uiSchema,omitempty"`
	Data            json.RawMessage             `json:"data,omitempty" yaml:"data,omitempty" apiredact:"secret"`
	Error           string                      `json:"error,omitempty" yaml:"error,omitempty"`
	CanRetry        bool                        `json:"canRetry,omitempty" yaml:"canRetry,omitempty"`
}

// ConnectionSetupAction returns the current or next setup observation. Its
// target is always the Connection being configured.
type ConnectionSetupAction struct {
	apiv1alpha1.Action[struct{}, ConnectionSetupActionStatus] `json:",inline" yaml:",inline"`
}

func (a *ConnectionSetupAction) ValidateResponse(expectedKind meta.Kind) error {
	if err := a.Action.ValidateResponse(expectedKind); err != nil {
		return err
	}
	if err := validateConnectionActionTarget(a.Metadata.Target); err != nil {
		return err
	}
	if a.Status == nil {
		return fmt.Errorf("$.status: is required")
	}

	switch a.Status.Type {
	case ConnectionSetupResponseTypeRedirect:
		if a.Status.RedirectURL == "" {
			return fmt.Errorf("$.status.redirectUrl: is required for a redirect setup result")
		}
	case ConnectionSetupResponseTypeForm:
		if a.Status.StepID == "" {
			return fmt.Errorf("$.status.stepId: is required for a form setup result")
		}
		if len(a.Status.JSONSchema) == 0 {
			return fmt.Errorf("$.status.jsonSchema: is required for a form setup result")
		}
		if len(a.Status.UISchema) == 0 {
			return fmt.Errorf("$.status.uiSchema: is required for a form setup result")
		}
	case ConnectionSetupResponseTypeComplete, ConnectionSetupResponseTypeVerifying:
	case ConnectionSetupResponseTypeError:
		if a.Status.Error == "" {
			return fmt.Errorf("$.status.error: is required for an error setup result")
		}
	default:
		return fmt.Errorf("$.status.type: is not a recognized setup result type")
	}
	return nil
}

func NewConnectionSetupAction(
	target meta.ObjectReference,
	status ConnectionSetupActionStatus,
) ConnectionSetupAction {
	return ConnectionSetupAction{Action: apiv1alpha1.NewActionResponse(
		ConnectionSetupActionKind,
		target,
		struct{}{},
		status,
	)}
}

// RedactConnectionSetupData returns a deep-copied setup-data object with every
// value masked. Setup submissions are write-only even for callers authorized
// to replay secrets from other resource types.
func RedactConnectionSetupData(data json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 {
		return nil, nil
	}

	value := struct {
		Data json.RawMessage `json:"data" apiredact:"secret"`
	}{Data: data}
	redacted, _, err := apserde.MarshalJSONForAPI(context.Background(), value)
	if err != nil {
		return nil, fmt.Errorf("redact connection setup data: %w", err)
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(redacted, &result); err != nil {
		return nil, fmt.Errorf("decode redacted connection setup data: %w", err)
	}
	return result.Data, nil
}

// ConnectionSetupSubmitSpec contains one setup-form submission. Data is a
// write-only secret-bearing object.
type ConnectionSetupSubmitSpec struct {
	StepID      string          `json:"stepId" yaml:"stepId"`
	Data        json.RawMessage `json:"data" yaml:"data" apiredact:"secret"`
	ReturnToURL string          `json:"returnToUrl,omitempty" yaml:"returnToUrl,omitempty"`
}

type ConnectionSetupSubmitAction struct {
	apiv1alpha1.Action[ConnectionSetupSubmitSpec, struct{}] `json:",inline" yaml:",inline"`
}

func (a *ConnectionSetupSubmitAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	if a.Spec.StepID == "" {
		return fmt.Errorf("$.spec.stepId: is required")
	}
	if len(a.Spec.Data) == 0 || string(a.Spec.Data) == "null" {
		return fmt.Errorf("$.spec.data: is required and must not be null")
	}
	return validateConnectionActionTarget(a.Metadata.Target)
}

// ConnectionSetupControlSpec is shared by retry and reauthentication actions.
type ConnectionSetupControlSpec struct {
	ReturnToURL string `json:"returnToUrl,omitempty" yaml:"returnToUrl,omitempty"`
}

type ConnectionSetupControlAction struct {
	apiv1alpha1.Action[ConnectionSetupControlSpec, struct{}] `json:",inline" yaml:",inline"`
}

func (a *ConnectionSetupControlAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return validateConnectionActionTarget(a.Metadata.Target)
}

type EmptyConnectionAction struct {
	apiv1alpha1.Action[struct{}, struct{}] `json:",inline" yaml:",inline"`
}

func (a *EmptyConnectionAction) ValidateRequest(expectedKind meta.Kind) error {
	if err := a.Action.ValidateRequest(expectedKind); err != nil {
		return err
	}
	return validateConnectionActionTarget(a.Metadata.Target)
}

func validateConnectionActionTarget(target meta.ObjectReference) error {
	vc := &common.ValidationContext{Path: "$.metadata.target"}
	if err := meta.ValidateObjectReferenceWithOptions(target, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       connectionschema.ConnectionKind,
		IDValidator:        connectionschema.ValidateID,
		NamespaceValidator: namespaceschema.ValidatePath,
	}, vc); err != nil {
		return err
	}
	if target.Generation != 0 {
		return vc.NewErrorForField("generation", "does not apply to connections")
	}
	return nil
}

// DataSourceOptionJson represents a single option from a setup data source.
type DataSourceOptionJson struct {
	Value string `json:"value" yaml:"value" example:"ws-123"`
	Label string `json:"label" yaml:"label" example:"My Workspace"`
}
