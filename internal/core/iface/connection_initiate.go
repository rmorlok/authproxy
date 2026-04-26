package iface

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type InitiateConnectionRequest struct {
	// Id of the connector to initiate the connector for
	ConnectorId apid.ID `json:"connector_id"`

	// Version of the connector to initiate connection for; if not specified defaults to the primary version.
	ConnectorVersion uint64 `json:"connector_version,omitempty"`

	// The namespace to create the connection in. Must be the namespace of connector or a child namespace of that
	// namespace. Defaults to the connector namespace if not specified.
	IntoNamespace string `json:"into_namespace,omitempty"`

	// The URL to return to after the connection is completed.
	ReturnToUrl string `json:"return_to_url"`
}

func (icr *InitiateConnectionRequest) Validate() error {
	result := &multierror.Error{}

	if icr.ConnectorId == apid.Nil {
		result = multierror.Append(result, fmt.Errorf("connector_id is required"))
	}

	if icr.HasIntoNamespace() {
		if err := aschema.ValidateNamespacePath(icr.IntoNamespace); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

func (icr *InitiateConnectionRequest) HasVersion() bool {
	return icr.ConnectorVersion > 0
}

func (icr *InitiateConnectionRequest) HasIntoNamespace() bool {
	return icr.IntoNamespace != ""
}

type ConnectionSetupResponseType string

const (
	ConnectionSetupResponseTypeRedirect  ConnectionSetupResponseType = "redirect"
	ConnectionSetupResponseTypeForm      ConnectionSetupResponseType = "form"
	ConnectionSetupResponseTypeComplete  ConnectionSetupResponseType = "complete"
	ConnectionSetupResponseTypeVerifying ConnectionSetupResponseType = "verifying"
	ConnectionSetupResponseTypeError     ConnectionSetupResponseType = "error"
)

type ConnectionSetupResponse interface {
	GetId() apid.ID
	GetType() ConnectionSetupResponseType
}

type ConnectionSetupRedirect struct {
	Id          apid.ID                     `json:"id"`
	Type        ConnectionSetupResponseType `json:"type"`
	RedirectUrl string                      `json:"redirect_url"`
}

func (icr *ConnectionSetupRedirect) GetId() apid.ID {
	return icr.Id
}

func (icr *ConnectionSetupRedirect) GetType() ConnectionSetupResponseType {
	return icr.Type
}

type ConnectionSetupForm struct {
	Id              apid.ID                     `json:"id"`
	Type            ConnectionSetupResponseType `json:"type"`
	StepId          string                      `json:"step_id"`
	StepTitle       string                      `json:"step_title,omitempty"`
	StepDescription string                      `json:"step_description,omitempty"`
	CurrentStep     int                         `json:"current_step"`
	TotalSteps      int                         `json:"total_steps"`
	JsonSchema      json.RawMessage             `json:"json_schema"`
	UiSchema        json.RawMessage             `json:"ui_schema"`
}

func (icf *ConnectionSetupForm) GetId() apid.ID {
	return icf.Id
}

func (icf *ConnectionSetupForm) GetType() ConnectionSetupResponseType {
	return icf.Type
}

type ConnectionSetupComplete struct {
	Id   apid.ID                     `json:"id"`
	Type ConnectionSetupResponseType `json:"type"`
}

func (icc *ConnectionSetupComplete) GetId() apid.ID {
	return icc.Id
}

func (icc *ConnectionSetupComplete) GetType() ConnectionSetupResponseType {
	return icc.Type
}

// ConnectionSetupVerifying indicates that probes are running in the background to verify
// the credentials obtained during auth. The UI should poll /_setup_step to observe the outcome.
type ConnectionSetupVerifying struct {
	Id   apid.ID                     `json:"id"`
	Type ConnectionSetupResponseType `json:"type"`
}

func (icv *ConnectionSetupVerifying) GetId() apid.ID {
	return icv.Id
}

func (icv *ConnectionSetupVerifying) GetType() ConnectionSetupResponseType {
	return icv.Type
}

// ConnectionSetupError is a terminal error response during setup, e.g. when probe verification
// fails. The UI should show the error and offer retry (POST /_retry) or cancel (POST /_abort).
type ConnectionSetupError struct {
	Id       apid.ID                     `json:"id"`
	Type     ConnectionSetupResponseType `json:"type"`
	Error    string                      `json:"error"`
	CanRetry bool                        `json:"can_retry"`
}

func (ice *ConnectionSetupError) GetId() apid.ID {
	return ice.Id
}

func (ice *ConnectionSetupError) GetType() ConnectionSetupResponseType {
	return ice.Type
}

type SubmitConnectionRequest struct {
	// StepId is the id of the step being submitted. Must match the current setup step's id.
	StepId string `json:"step_id"`

	// Data is the form data submitted by the user for the current step.
	Data json.RawMessage `json:"data"`

	// ReturnToUrl is required when the next step after form submission is an auth redirect.
	// The client should provide this so the OAuth callback knows where to return.
	ReturnToUrl string `json:"return_to_url,omitempty"`
}
