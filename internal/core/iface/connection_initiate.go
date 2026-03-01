package iface

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/hashicorp/go-multierror"
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

type InitiateConnectionResponseType string

const (
	PreconnectionResponseTypeRedirect InitiateConnectionResponseType = "redirect"
)

type InitiateConnectionResponse interface {
	GetId() apid.ID
	GetType() InitiateConnectionResponseType
}

type InitiateConnectionRedirect struct {
	Id          apid.ID                      `json:"id"`
	Type        InitiateConnectionResponseType `json:"type"`
	RedirectUrl string                         `json:"redirect_url"`
}

func (icr *InitiateConnectionRedirect) GetId() apid.ID {
	return icr.Id
}

func (icr *InitiateConnectionRedirect) GetType() InitiateConnectionResponseType {
	return icr.Type
}
