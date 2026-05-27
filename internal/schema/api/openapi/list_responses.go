package openapi

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
)

// ListActorsResponseJson documents the paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	// List of actors.
	Items []schemaapi.ActorJson `json:"items"`
	// Pagination cursor for next page.
	Cursor string `json:"cursor,omitempty"`
}

// ListNamespacesResponseJson documents the paginated namespace list response.
//
//	@Description	Paginated list of namespaces
type ListNamespacesResponseJson struct {
	// List of namespaces.
	Items []schemaapi.NamespaceJson `json:"items"`
	// Pagination cursor for next page.
	Cursor string `json:"cursor,omitempty"`
}

// ListConnectorsResponseJson documents the paginated connector list response.
//
//	@Description	Paginated list of connectors
type ListConnectorsResponseJson struct {
	// List of connectors.
	Items []schemaapi.ConnectorJson `json:"items"`
	// Pagination cursor for next page.
	Cursor string `json:"cursor,omitempty"`
}

// ConnectorVersionJson documents a connector version response.
//
//	@Description	Detailed connector version information
type ConnectorVersionJson struct {
	Id          apid.ID                         `json:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version     uint64                          `json:"version" example:"1"`
	Namespace   string                          `json:"namespace" example:"root.acme"`
	State       schemaapi.ConnectorVersionState `json:"state" swaggertype:"string" example:"primary"`
	Definition  interface{}                     `json:"definition"`
	Labels      map[string]string               `json:"labels,omitempty"`
	Annotations map[string]string               `json:"annotations,omitempty"`
	CreatedAt   time.Time                       `json:"created_at"`
	UpdatedAt   time.Time                       `json:"updated_at"`
}

// ListConnectorVersionsResponseJson documents the paginated connector version list response.
//
//	@Description	Paginated list of connector versions
type ListConnectorVersionsResponseJson struct {
	// List of connector versions.
	Items []interface{} `json:"items"`
	// Pagination cursor for next page.
	Cursor string `json:"cursor,omitempty"`
}

// CreateConnectorRequestJson documents the connector creation body.
//
//	@Description	Request to create a new connector
type CreateConnectorRequestJson struct {
	Namespace   string            `json:"namespace" example:"root.acme"`
	Definition  interface{}       `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// UpdateConnectorRequestJson documents the connector update body.
//
//	@Description	Request to update a connector or connector version
type UpdateConnectorRequestJson struct {
	Definition  interface{}        `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// CreateConnectorVersionRequestJson documents the connector version creation body.
//
//	@Description	Request to create a new draft connector version
type CreateConnectorVersionRequestJson struct {
	Definition  interface{}        `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}
