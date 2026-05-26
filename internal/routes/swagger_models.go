package routes

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
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

type InitiateConnectionRequest = schemaapi.InitiateConnectionRequest
type ConnectionSetupRedirect = schemaapi.ConnectionSetupRedirect
type ConnectionSetupForm = schemaapi.ConnectionSetupForm
type ConnectionSetupComplete = schemaapi.ConnectionSetupComplete
type SubmitConnectionRequest = schemaapi.SubmitConnectionRequest
type DataSourceOptionJson = schemaapi.DataSourceOptionJson
type SwaggerNamespaceJson = schemaapi.NamespaceJson
type SwaggerConnectorJson = schemaapi.ConnectorJson
type SwaggerListConnectorsResponse = schemaapiopenapi.ListConnectorsResponseJson
type SwaggerConnectorVersionJson = schemaapiopenapi.ConnectorVersionJson
type SwaggerListConnectorVersionsResponse = schemaapiopenapi.ListConnectorVersionsResponseJson
type SwaggerCreateConnectorRequest = schemaapiopenapi.CreateConnectorRequestJson
type SwaggerUpdateConnectorRequest = schemaapiopenapi.UpdateConnectorRequestJson
type SwaggerCreateConnectorVersionRequest = schemaapiopenapi.CreateConnectorVersionRequestJson

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
	// Optional labels to attach to this request's log entry (merged with connection labels; request labels override)
	Labels map[string]string `json:"labels,omitempty"`
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
	Id apid.ID `swaggertype:"string" json:"id" example:"req_test550e8400abcde"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// Labels assigned to the connection
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations assigned to the connection
	Annotations map[string]string `json:"annotations,omitempty"`
	// Connection state (pending, connected, disconnecting, disconnected, error)
	State string `json:"state" example:"connected"`
	// Operational health signal (healthy, unhealthy). Distinct from State: a Ready connection
	// whose credentials have stopped working flips to unhealthy without changing State.
	HealthState string `json:"health_state" example:"healthy"`
	// Current setup step if connection setup is in progress
	SetupStep *string `json:"setup_step,omitempty" example:"preconnect:0"`
	// Connector information
	Connector SwaggerConnectorJson `json:"connector"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerRequestEventsEntry is a simplified request events entry for swagger documentation
//
//	@Description	HTTP request events entry
type SwaggerRequestEventsEntry struct {
	// Namespace of the connection
	Namespace string `json:"namespace" example:"acme"`
	// Request type (proxy, oauth, probe)
	Type string `json:"type" example:"proxy"`
	// Request UUID
	RequestId apid.ID `swaggertype:"string" json:"request_id" example:"req_test550e8400abcde"`
	// Correlation ID for tracing
	CorrelationId string `json:"correlation_id,omitempty"`
	// Request timestamp
	Timestamp time.Time `json:"timestamp"`
	// Duration in milliseconds
	Duration int64 `json:"duration" example:"150"`
	// Connection UUID
	ConnectionId apid.ID `swaggertype:"string" json:"connection_id,omitempty"`
	// Connector UUID
	ConnectorId apid.ID `swaggertype:"string" json:"connector_id,omitempty"`
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
	// Labels associated with the request (merged from connection and per-request labels)
	Labels map[string]string `json:"labels,omitempty"`
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

// SwaggerEncryptionKeyJson is a simplified encryption key model for swagger documentation
//
//	@Description	Encryption key configuration
type SwaggerEncryptionKeyJson struct {
	// Encryption key ID
	Id apid.ID `swaggertype:"string" json:"id" example:"ek_test550e8400abcde"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// State (active, disabled)
	State string `json:"state" example:"active"`
	// Labels assigned to the encryption key
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations assigned to the encryption key
	Annotations map[string]string `json:"annotations,omitempty"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerListEncryptionKeysResponse is the response for list encryption keys
//
//	@Description	Paginated list of encryption keys
type SwaggerListEncryptionKeysResponse struct {
	// List of encryption keys
	Items []SwaggerEncryptionKeyJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerRateLimitJson is a simplified rate-limit model for swagger documentation.
//
//	@Description	Rate limit configuration
type SwaggerRateLimitJson struct {
	// Rate limit ID
	Id apid.ID `swaggertype:"string" json:"id" example:"rl_test550e8400abcde"`
	// Namespace path
	Namespace string `json:"namespace" example:"acme"`
	// JSON-serialised definition (mode, selector, bucket, algorithm)
	Definition map[string]interface{} `json:"definition"`
	// Labels assigned to the rate limit
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations assigned to the rate limit
	Annotations map[string]string `json:"annotations,omitempty"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// SwaggerListRateLimitsResponse is the response for list rate limits.
//
//	@Description	Paginated list of rate limits
type SwaggerListRateLimitsResponse struct {
	// List of rate limits
	Items []SwaggerRateLimitJson `json:"items"`
	// Pagination cursor for next page
	Cursor string `json:"cursor,omitempty"`
}

// SwaggerDryRunRequest is the input for the rate-limit dry-run endpoint.
// Reuses ProxyRequest so the request body is identical to the shape the
// real /connections/{id}/_proxy endpoint accepts.
//
//	@Description	Dry-run input: a proxy-shaped request + request type + the identity it runs under
type SwaggerDryRunRequest struct {
	Request     ProxyRequest         `json:"request"`
	RequestType string               `json:"request_type" example:"proxy"`
	Context     SwaggerDryRunContext `json:"context"`
}

// SwaggerDryRunContext is the actor + connection + namespace identity.
// Labels live on Request now (matching ProxyRequest).
//
//	@Description	Identity the request runs under
type SwaggerDryRunContext struct {
	ConnectionId string `json:"connection_id,omitempty"`
	ActorId      string `json:"actor_id,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
}

// SwaggerDryRunResponse is what the endpoint returns.
//
//	@Description	Per-rule match + peek-driven would-allow result
type SwaggerDryRunResponse struct {
	RequestLabelSnapshot map[string]string         `json:"request_label_snapshot"`
	Matched              []SwaggerDryRunMatch      `json:"matched"`
	NotMatched           []SwaggerDryRunNotMatched `json:"not_matched"`
}

type SwaggerDryRunMatch struct {
	RateLimitId      string `json:"rate_limit_id" swaggertype:"string" example:"rl_test550e8400"`
	Namespace        string `json:"namespace"`
	EffectiveMode    string `json:"effective_mode" example:"enforce"`
	BucketKey        string `json:"bucket_key" example:"actor=act_abc|labels/team=acme"`
	AlgorithmSummary string `json:"algorithm_summary" example:"token bucket 60 @ 1/s"`
	WouldAllow       bool   `json:"would_allow"`
	Remaining        int    `json:"remaining"`
	RetryAfterMs     int64  `json:"retry_after_ms"`
	PeekFailed       bool   `json:"peek_failed"`
}

type SwaggerDryRunNotMatched struct {
	RateLimitId string `json:"rate_limit_id" swaggertype:"string"`
	Namespace   string `json:"namespace"`
	Reason      string `json:"reason"`
}

// SwaggerListRequestEventsResponse is the response for listing request events
//
//	@Description	Paginated list of request events entries
type SwaggerListRequestEventsResponse struct {
	// List of request events entries
	Items []SwaggerRequestEventsEntry `json:"items"`
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

// SwaggerForceConnectorVersionStateRequest is the request to force a connector version state
//
//	@Description	Request to force a connector version state
type SwaggerForceConnectorVersionStateRequest struct {
	// Target state (draft, primary, active, archived)
	State string `json:"state" example:"primary"`
}

// SwaggerUpdateConnectionRequest is the request to update a connection
//
//	@Description	Request to update a connection's labels
type SwaggerUpdateConnectionRequest struct {
	// Labels to set on the connection (replaces all existing labels)
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations to set on the connection (replaces all existing annotations)
	Annotations map[string]string `json:"annotations,omitempty"`
}

// SwaggerKeyValueJson is a single key-value pair, used by both label
// and annotation endpoints across all resource types.
//
//	@Description	Key-value pair (label or annotation)
type SwaggerKeyValueJson struct {
	// Key
	Key string `json:"key" example:"env"`
	// Value
	Value string `json:"value" example:"production"`
}

// SwaggerPutKeyValueRequest is the body for PUT label/annotation
// endpoints across all resource types.
//
//	@Description	Request to set a label or annotation value
type SwaggerPutKeyValueRequest struct {
	// Value to set
	Value string `json:"value" example:"production"`
}
