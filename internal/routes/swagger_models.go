package routes

import (
	"time"

	"github.com/google/uuid"
)

// ErrorResponse is the standardized error response format for authproxy API errors.
//
//	@Description	Standardized error response
type ErrorResponse struct {
	// Error message
	Error string `json:"error" example:"Bad Request"`
	// Stack trace (only in debug mode)
	StackTrace string `json:"stack_trace,omitempty"`
}

// InitiateConnectionRequest represents a request to initiate a connection to an external service.
//
//	@Description	Request to initiate a connection
type InitiateConnectionRequest struct {
	// ID of the connector to initiate the connection for
	ConnectorId uuid.UUID `json:"connector_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Version of the connector (optional, defaults to primary version)
	ConnectorVersion uint64 `json:"connector_version,omitempty" example:"1"`
	// Namespace to create the connection in (optional, defaults to connector namespace)
	IntoNamespace string `json:"into_namespace,omitempty" example:"acme"`
	// URL to return to after the connection is completed
	ReturnToUrl string `json:"return_to_url" example:"https://example.com/callback"`
}

// InitiateConnectionRedirect represents the response when a connection requires a redirect for OAuth.
//
//	@Description	Redirect response for connection initiation
type InitiateConnectionRedirect struct {
	// Connection UUID
	Id uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Response type (always "redirect")
	Type string `json:"type" example:"redirect"`
	// URL to redirect the user to
	RedirectUrl string `json:"redirect_url" example:"https://oauth.provider.com/authorize?..."`
}

// ProxyRequest represents a request to proxy through a connection.
//
//	@Description	Request to proxy an HTTP request through a connection
type ProxyRequest struct {
	// Target URL to proxy to
	URL string `json:"url" example:"https://api.example.com/v1/users"`
	// HTTP method
	Method string `json:"method" example:"GET"`
	// HTTP headers
	Headers map[string]string `json:"headers,omitempty"`
	// Raw body bytes (base64 encoded if binary)
	BodyRaw []byte `json:"body_raw,omitempty"`
	// JSON body (alternative to body_raw)
	BodyJson interface{} `json:"body_json,omitempty"`
}

// ProxyResponse represents the response from a proxied request.
//
//	@Description	Response from a proxied HTTP request
type ProxyResponse struct {
	// HTTP status code
	StatusCode int `json:"status_code" example:"200"`
	// Response headers
	Headers map[string]string `json:"headers,omitempty"`
	// Raw response body bytes
	BodyRaw []byte `json:"body_raw,omitempty"`
	// JSON response body (if content-type is application/json)
	BodyJson interface{} `json:"body_json,omitempty"`
}

// SwaggerConnectionJson is a simplified connection model for swagger documentation
//
//	@Description	Connection to an external service
type SwaggerConnectionJson struct {
	// Connection UUID
	Id uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// Labels assigned to the connection
	Labels map[string]string `json:"labels,omitempty"`
	// Connection state (pending, connected, disconnecting, disconnected, error)
	State string `json:"state" example:"connected"`
	// Connector information
	Connector SwaggerConnectorJson `json:"connector"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerConnectorJson is a simplified connector model for swagger documentation
//
//	@Description	Connector definition for external service integration
type SwaggerConnectorJson struct {
	// Connector UUID
	Id uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Connector version number
	Version uint64 `json:"version" example:"1"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// State (draft, active, deprecated, archived)
	State string `json:"state" example:"active"`
	// Display name
	DisplayName string `json:"display_name" example:"Salesforce"`
	// Short highlight text
	Highlight string `json:"highlight,omitempty" example:"CRM platform"`
	// Full description
	Description string `json:"description" example:"Salesforce CRM integration"`
	// Logo URL
	Logo string `json:"logo,omitempty" example:"https://example.com/logo.png"`
	// Labels assigned to the connector
	Labels map[string]string `json:"labels,omitempty"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerNamespaceJson is a simplified namespace model for swagger documentation
//
//	@Description	Namespace for organizing resources
type SwaggerNamespaceJson struct {
	// Namespace path (e.g., "acme" or "acme/sales")
	Path string `json:"path" example:"acme"`
	// Namespace state (active, suspended)
	State string `json:"state" example:"active"`
	// Labels assigned to the namespace
	Labels map[string]string `json:"labels,omitempty"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerRequestLogEntry is a simplified request log entry for swagger documentation
//
//	@Description	HTTP request log entry
type SwaggerRequestLogEntry struct {
	// Namespace of the connection
	Namespace string `json:"namespace" example:"acme"`
	// Request type (proxy, oauth, probe)
	Type string `json:"type" example:"proxy"`
	// Request UUID
	RequestId uuid.UUID `json:"request_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Correlation ID for tracing
	CorrelationId string `json:"correlation_id,omitempty"`
	// Request timestamp
	Timestamp time.Time `json:"timestamp"`
	// Duration in milliseconds
	Duration int64 `json:"duration" example:"150"`
	// Connection UUID
	ConnectionId uuid.UUID `json:"connection_id,omitempty"`
	// Connector UUID
	ConnectorId uuid.UUID `json:"connector_id,omitempty"`
	// Connector version
	ConnectorVersion uint64 `json:"connector_version,omitempty"`
	// HTTP method
	Method string `json:"method" example:"GET"`
	// Target host
	Host string `json:"host" example:"api.example.com"`
	// URL scheme
	Scheme string `json:"scheme" example:"https"`
	// Request path
	Path string `json:"path" example:"/v1/users"`
	// HTTP response status code
	ResponseStatusCode int `json:"response_status_code" example:"200"`
}

// SwaggerListConnectionResponse is the response for list connections
//
//	@Description	Paginated list of connections
type SwaggerListConnectionResponse struct {
	// List of connections
	Items []SwaggerConnectionJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerDisconnectResponse is the response for disconnect operation
//
//	@Description	Response for disconnect operation
type SwaggerDisconnectResponse struct {
	// Task ID for tracking the disconnect operation
	TaskId string `json:"task_id"`
	// Connection being disconnected
	Connection SwaggerConnectionJson `json:"connection"`
}

// SwaggerListConnectorsResponse is the response for list connectors
//
//	@Description	Paginated list of connectors
type SwaggerListConnectorsResponse struct {
	// List of connectors
	Items []SwaggerConnectorJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerConnectorVersionJson is the detailed connector version
//
//	@Description	Detailed connector version information
type SwaggerConnectorVersionJson struct {
	// Connector UUID
	Id uuid.UUID `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Connector version number
	Version uint64 `json:"version" example:"1"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// State (draft, active, deprecated, archived)
	State string `json:"state" example:"active"`
	// Connector definition (full schema)
	Definition interface{} `json:"definition"`
	// Labels assigned to the connector
	Labels map[string]string `json:"labels,omitempty"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerListConnectorVersionsResponse is the response for list connector versions
//
//	@Description	Paginated list of connector versions
type SwaggerListConnectorVersionsResponse struct {
	// List of connector versions
	Items []SwaggerConnectorVersionJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerListNamespacesResponse is the response for list namespaces
//
//	@Description	Paginated list of namespaces
type SwaggerListNamespacesResponse struct {
	// List of namespaces
	Items []SwaggerNamespaceJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerListRequestsResponse is the response for list request logs
//
//	@Description	Paginated list of request log entries
type SwaggerListRequestsResponse struct {
	// List of request log entries
	Items []SwaggerRequestLogEntry `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
	// Total count of matching records (if requested)
	Total *int64 `json:"total,omitempty"`
}

// SwaggerForceStateRequest is the request to force a connection state
//
//	@Description	Request to force a connection state
type SwaggerForceStateRequest struct {
	// Target state (pending, connected, disconnecting, disconnected, error)
	State string `json:"state" example:"connected"`
}
