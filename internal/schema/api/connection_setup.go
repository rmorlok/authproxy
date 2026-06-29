package api

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// InitiateConnectionRequest represents a request to initiate a connection to an external service.
//
//	@Description	Request to initiate a connection
type InitiateConnectionRequest struct {
	// ID of the connector to initiate the connection for.
	ConnectorId apid.ID `json:"connector_id" yaml:"connector_id" swaggertype:"string" example:"cxr_test550e8400abcde"`

	// Version of the connector to initiate connection for; if not specified defaults to the primary version.
	ConnectorVersion uint64 `json:"connector_version,omitempty" yaml:"connector_version,omitempty" example:"1"`

	// The namespace to create the connection in. Must be the namespace of connector or a child namespace of that
	// namespace. Defaults to the connector namespace if not specified.
	IntoNamespace string `json:"into_namespace,omitempty" yaml:"into_namespace,omitempty" example:"root.acme"`

	// The URL to return to after the connection is completed.
	ReturnToUrl string `json:"return_to_url" yaml:"return_to_url" example:"https://example.com/callback"`
}

func (icr *InitiateConnectionRequest) Validate() error {
	result := &multierror.Error{}

	if icr.ConnectorId == apid.Nil {
		result = multierror.Append(result, fmt.Errorf("connector_id is required"))
	}

	if icr.HasIntoNamespace() {
		if err := nschema.ValidatePath(icr.IntoNamespace); err != nil {
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

// ConnectionSetupRedirect represents the response when a connection requires a redirect for OAuth.
//
//	@Description	Redirect response for connection setup
type ConnectionSetupRedirect struct {
	// Connection UUID.
	Id apid.ID `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`

	// Response type.
	Type ConnectionSetupResponseType `json:"type" yaml:"type" swaggertype:"string" example:"redirect"`

	// URL to redirect the user to.
	RedirectUrl string `json:"redirect_url" yaml:"redirect_url" example:"https://oauth.provider.com/authorize?..."`
}

func (icr *ConnectionSetupRedirect) GetId() apid.ID {
	return icr.Id
}

func (icr *ConnectionSetupRedirect) GetType() ConnectionSetupResponseType {
	return icr.Type
}

// ConnectionSetupForm represents the response when a connection requires form input.
//
//	@Description	Form response for connection setup
type ConnectionSetupForm struct {
	// Connection UUID.
	Id apid.ID `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`

	// Response type.
	Type ConnectionSetupResponseType `json:"type" yaml:"type" swaggertype:"string" example:"form"`

	// Step ID being submitted.
	StepId string `json:"step_id" yaml:"step_id" example:"preconnect:0"`

	// Step title.
	StepTitle string `json:"step_title,omitempty" yaml:"step_title,omitempty" example:"Choose workspace"`

	// Step description.
	StepDescription string `json:"step_description,omitempty" yaml:"step_description,omitempty"`

	// JSON Schema defining the form fields.
	JsonSchema json.RawMessage `json:"json_schema" yaml:"json_schema" swaggertype:"object"`

	// UI Schema for JSON Forms rendering.
	UiSchema json.RawMessage `json:"ui_schema" yaml:"ui_schema" swaggertype:"object"`
}

func (icf *ConnectionSetupForm) GetId() apid.ID {
	return icf.Id
}

func (icf *ConnectionSetupForm) GetType() ConnectionSetupResponseType {
	return icf.Type
}

// ConnectionSetupComplete represents the response when a connection setup is complete.
//
//	@Description	Completion response for connection setup
type ConnectionSetupComplete struct {
	// Connection UUID.
	Id apid.ID `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`

	// Response type.
	Type ConnectionSetupResponseType `json:"type" yaml:"type" swaggertype:"string" example:"complete"`
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
	Id   apid.ID                     `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Type ConnectionSetupResponseType `json:"type" yaml:"type" swaggertype:"string" example:"verifying"`
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
	Id       apid.ID                     `json:"id" yaml:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Type     ConnectionSetupResponseType `json:"type" yaml:"type" swaggertype:"string" example:"error"`
	Error    string                      `json:"error" yaml:"error" example:"probe verification failed"`
	CanRetry bool                        `json:"can_retry" yaml:"can_retry" example:"true"`
}

func (ice *ConnectionSetupError) GetId() apid.ID {
	return ice.Id
}

func (ice *ConnectionSetupError) GetType() ConnectionSetupResponseType {
	return ice.Type
}

// SubmitConnectionRequest represents a form data submission for a connection setup step.
//
//	@Description	Form submission data
type SubmitConnectionRequest struct {
	// StepId is the id of the step being submitted. Must match the current setup step's id.
	StepId string `json:"step_id" yaml:"step_id" example:"preconnect:0"`

	// Data is the form data submitted by the user for the current step.
	Data json.RawMessage `json:"data" yaml:"data" swaggertype:"object"`

	// ReturnToUrl is required when the next step after form submission is an auth redirect.
	// The client should provide this so the OAuth callback knows where to return.
	ReturnToUrl string `json:"return_to_url,omitempty" yaml:"return_to_url,omitempty" example:"https://example.com/callback"`
}

// DataSourceOptionJson represents a single option from a data source for populating form dropdowns.
//
//	@Description	Data source option for form select fields
type DataSourceOptionJson struct {
	// Option value.
	Value string `json:"value" yaml:"value" example:"ws-123"`

	// Human-readable label.
	Label string `json:"label" yaml:"label" example:"My Workspace"`
}
