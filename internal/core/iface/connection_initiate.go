package iface

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// InitiateConnectionRequest is the core command used to start setup. HTTP
// transports convert their versioned action spec into this internal type.
type InitiateConnectionRequest struct {
	ConnectorRef  meta.ObjectReference
	IntoNamespace string
	Name          *common.ResourceName
	Labels        map[string]string
	Annotations   map[string]string
	ReturnToUrl   string
}

func (r *InitiateConnectionRequest) Validate() error {
	var result *multierror.Error
	if err := meta.ValidateObjectReferenceWithOptions(r.ConnectorRef, meta.ObjectReferenceValidationOptions{
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       connectorschema.ConnectorKind,
		IDValidator:        connectorschema.ValidateID,
		NamespaceValidator: nschema.ValidatePath,
	}, nil); err != nil {
		result = multierror.Append(result, fmt.Errorf("connector reference is invalid: %w", err))
	}
	if r.HasIntoNamespace() {
		if err := nschema.ValidatePath(r.IntoNamespace); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if r.Name != nil {
		if err := r.Name.Validate(); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid connection name: %w", err))
		}
	}
	if err := meta.ValidateUserLabels(r.Labels); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection labels: %w", err))
	}
	if err := meta.ValidateAnnotations(r.Annotations); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connection annotations: %w", err))
	}
	return result.ErrorOrNil()
}

func (r *InitiateConnectionRequest) HasIntoNamespace() bool {
	return r.IntoNamespace != ""
}

type ConnectionSetupResponseType string

const (
	ConnectionSetupResponseTypeRedirect  ConnectionSetupResponseType = "redirect"
	ConnectionSetupResponseTypeForm      ConnectionSetupResponseType = "form"
	ConnectionSetupResponseTypeComplete  ConnectionSetupResponseType = "complete"
	ConnectionSetupResponseTypeVerifying ConnectionSetupResponseType = "verifying"
	ConnectionSetupResponseTypeError     ConnectionSetupResponseType = "error"
)

// ConnectionSetupResponse is an internal setup outcome. Routes turn it into a
// versioned ConnectionSetup action response.
type ConnectionSetupResponse interface {
	GetId() apid.ID
	GetType() ConnectionSetupResponseType
}

type ConnectionSetupRedirect struct {
	Id          apid.ID
	Type        ConnectionSetupResponseType
	RedirectUrl string
}

func (r *ConnectionSetupRedirect) GetId() apid.ID                       { return r.Id }
func (r *ConnectionSetupRedirect) GetType() ConnectionSetupResponseType { return r.Type }

type ConnectionSetupForm struct {
	Id              apid.ID
	Type            ConnectionSetupResponseType
	StepId          string
	StepTitle       string
	StepDescription string
	JsonSchema      json.RawMessage
	UiSchema        json.RawMessage
	Data            json.RawMessage
}

func (r *ConnectionSetupForm) GetId() apid.ID                       { return r.Id }
func (r *ConnectionSetupForm) GetType() ConnectionSetupResponseType { return r.Type }

type ConnectionSetupComplete struct {
	Id   apid.ID
	Type ConnectionSetupResponseType
}

func (r *ConnectionSetupComplete) GetId() apid.ID                       { return r.Id }
func (r *ConnectionSetupComplete) GetType() ConnectionSetupResponseType { return r.Type }

type ConnectionSetupVerifying struct {
	Id   apid.ID
	Type ConnectionSetupResponseType
}

func (r *ConnectionSetupVerifying) GetId() apid.ID                       { return r.Id }
func (r *ConnectionSetupVerifying) GetType() ConnectionSetupResponseType { return r.Type }

type ConnectionSetupError struct {
	Id       apid.ID
	Type     ConnectionSetupResponseType
	Error    string
	CanRetry bool
}

func (r *ConnectionSetupError) GetId() apid.ID                       { return r.Id }
func (r *ConnectionSetupError) GetType() ConnectionSetupResponseType { return r.Type }

type SubmitConnectionRequest struct {
	StepId      string
	Data        json.RawMessage
	ReturnToUrl string
}
