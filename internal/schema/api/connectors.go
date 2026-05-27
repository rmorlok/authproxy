package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// ConnectorVersionState is the API-visible lifecycle state of a connector version.
type ConnectorVersionState string

const (
	ConnectorVersionStateDraft    ConnectorVersionState = "draft"
	ConnectorVersionStatePrimary  ConnectorVersionState = "primary"
	ConnectorVersionStateActive   ConnectorVersionState = "active"
	ConnectorVersionStateArchived ConnectorVersionState = "archived"
)

type ConnectorVersionStates []ConnectorVersionState

// ConnectorJson represents the API summary projection of a connector version.
//
//	@Description	Connector API summary response
type ConnectorJson struct {
	Id            apid.ID                `json:"id" yaml:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version       uint64                 `json:"version" yaml:"version" example:"1"`
	Namespace     string                 `json:"namespace" yaml:"namespace" example:"root.acme"`
	State         ConnectorVersionState  `json:"state" yaml:"state" swaggertype:"string" example:"primary"`
	DisplayName   string                 `json:"display_name" yaml:"display_name" example:"Salesforce"`
	Highlight     string                 `json:"highlight,omitempty" yaml:"highlight,omitempty" example:"CRM platform"`
	Description   string                 `json:"description" yaml:"description" example:"Salesforce CRM integration"`
	StatusPageUrl string                 `json:"status_page_url,omitempty" yaml:"status_page_url,omitempty" example:"https://status.salesforce.com"`
	Logo          string                 `json:"logo" yaml:"logo" example:"https://example.com/logo.png"`
	HasConfigure  bool                   `json:"has_configure" yaml:"has_configure" example:"false"`
	Labels        map[string]string      `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations   map[string]string      `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt     time.Time              `json:"created_at" yaml:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" yaml:"updated_at"`
	Versions      int64                  `json:"versions,omitempty" yaml:"versions,omitempty" example:"2"`
	States        ConnectorVersionStates `json:"states,omitempty" yaml:"states,omitempty" swaggertype:"array,string"`
}

type ListConnectorsResponseJson struct {
	Items  []ConnectorJson `json:"items" yaml:"items"`
	Cursor string          `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// ConnectorVersionJson represents a single connector version returned by the API.
//
//	@Description	Detailed connector version information
type ConnectorVersionJson struct {
	Id          apid.ID               `json:"id" yaml:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version     uint64                `json:"version" yaml:"version" example:"1"`
	Namespace   string                `json:"namespace" yaml:"namespace" example:"root.acme"`
	State       ConnectorVersionState `json:"state" yaml:"state" swaggertype:"string" example:"primary"`
	Definition  cschema.Connector     `json:"definition" yaml:"definition"`
	Labels      map[string]string     `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string     `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CreatedAt   time.Time             `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at" yaml:"updated_at"`
}

type ListConnectorVersionsResponseJson struct {
	Items  []ConnectorVersionJson `json:"items" yaml:"items"`
	Cursor string                 `json:"cursor,omitempty" yaml:"cursor,omitempty"`
}

// CreateConnectorRequestJson is the request body for POST /connectors.
//
//	@Description	Request to create a new connector
type CreateConnectorRequestJson struct {
	Namespace   string            `json:"namespace" yaml:"namespace" example:"root.acme"`
	Definition  cschema.Connector `json:"definition" yaml:"definition"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// UpdateConnectorRequestJson is the request body for PATCH /connectors/:id and PATCH /connectors/:id/versions/:version.
//
//	@Description	Request to update a connector or connector version
type UpdateConnectorRequestJson struct {
	Definition  *cschema.Connector `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// CreateConnectorVersionRequestJson is the request body for POST /connectors/:id/versions.
//
//	@Description	Request to create a new draft connector version
type CreateConnectorVersionRequestJson struct {
	Definition  *cschema.Connector `json:"definition,omitempty" yaml:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}
