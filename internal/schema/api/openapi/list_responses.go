package openapi

import schemaapi "github.com/rmorlok/authproxy/internal/schema/api"

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
