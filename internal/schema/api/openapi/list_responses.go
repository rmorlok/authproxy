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

// ListEncryptionKeysResponseJson documents the paginated encryption-key list response.
//
//	@Description	Paginated list of encryption keys
type ListEncryptionKeysResponseJson struct {
	Items  []schemaapi.EncryptionKeyJson `json:"items"`
	Cursor string                        `json:"cursor,omitempty"`
}

// ListRequestEventsResponseJson documents the paginated request-events list response.
//
//	@Description	Paginated list of request events entries
type ListRequestEventsResponseJson struct {
	Items  []interface{} `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
	Total  *int64        `json:"total,omitempty"`
}

// RequestEventJson documents the public request-event record projection.
//
//	@Description	HTTP request events entry
type RequestEventJson struct {
	Namespace           string            `json:"namespace" example:"root.acme"`
	Type                string            `json:"type" example:"proxy"`
	RequestId           string            `json:"request_id" swaggertype:"string" example:"req_test550e8400abcde"`
	CorrelationId       string            `json:"correlation_id,omitempty"`
	Timestamp           time.Time         `json:"timestamp"`
	MillisecondDuration int64             `json:"duration" example:"150"`
	ConnectionId        string            `json:"connection_id,omitempty" swaggertype:"string"`
	ConnectorId         string            `json:"connector_id,omitempty" swaggertype:"string"`
	ConnectorVersion    uint64            `json:"connector_version,omitempty"`
	Method              string            `json:"method" example:"GET"`
	Host                string            `json:"host" example:"api.example.com"`
	Scheme              string            `json:"scheme" example:"https"`
	Path                string            `json:"path" example:"/v1/users"`
	ResponseStatusCode  int               `json:"response_status_code,omitempty" example:"200"`
	Labels              map[string]string `json:"labels,omitempty"`
	ResponseSource      string            `json:"response_source,omitempty" example:"upstream"`
	RateLimitId         string            `json:"rate_limit_id,omitempty" swaggertype:"string"`
	RateLimitMode       string            `json:"rate_limit_mode,omitempty"`
	RateLimitBucket     map[string]string `json:"rate_limit_bucket,omitempty"`
	RateLimitMatched    []interface{}     `json:"rate_limit_matched,omitempty"`
}

// TaskInfoJson documents public background task status.
//
//	@Description	Background task status
type TaskInfoJson struct {
	Id        string `json:"id"`
	Type      string `json:"type"`
	State     string `json:"state" example:"completed"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// UpdateEncryptionKeyRequestJson documents the encryption-key update body.
//
//	@Description	Request to update an encryption key
type UpdateEncryptionKeyRequestJson struct {
	State       *string            `json:"state,omitempty" example:"disabled"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// RateLimitJson documents a rate-limit response while keeping the definition
// opaque for swaggo.
//
//	@Description	Rate-limit API response
type RateLimitJson struct {
	Id          apid.ID           `json:"id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string            `json:"namespace" example:"root.acme"`
	Definition  map[string]any    `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ListRateLimitsResponseJson documents the paginated rate-limit list response.
//
//	@Description	Paginated list of rate limits
type ListRateLimitsResponseJson struct {
	Items  []interface{} `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
}

// CreateRateLimitRequestJson documents the rate-limit creation body.
//
//	@Description	Request to create a rate limit
type CreateRateLimitRequestJson struct {
	Namespace   string            `json:"namespace" example:"root.acme"`
	Definition  map[string]any    `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// UpdateRateLimitRequestJson documents the rate-limit update body.
//
//	@Description	Request to update a rate limit
type UpdateRateLimitRequestJson struct {
	Definition  map[string]any     `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// ProxyRequestJson documents the proxy-shaped request used by dry-run.
//
//	@Description	Request to simulate an HTTP request
type ProxyRequestJson struct {
	URL      string            `json:"url" example:"https://api.example.com/v1/users"`
	Method   string            `json:"method" example:"POST"`
	Headers  map[string]string `json:"headers,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	BodyRaw  []byte            `json:"body_raw,omitempty"`
	BodyJson interface{}       `json:"body_json,omitempty"`
}

// DryRunRequestJson documents the rate-limit dry-run request body.
//
//	@Description	Dry-run input: a proxy-shaped request + request type + the identity it runs under
type DryRunRequestJson struct {
	Request     interface{} `json:"request"`
	RequestType string      `json:"request_type" example:"proxy"`
	Context     interface{} `json:"context"`
}

// DryRunContextJson documents the dry-run identity context.
//
//	@Description	Identity the request runs under
type DryRunContextJson struct {
	ConnectionId string `json:"connection_id,omitempty"`
	ActorId      string `json:"actor_id,omitempty"`
	Namespace    string `json:"namespace,omitempty" example:"root.acme"`
}

// DryRunResponseJson documents the dry-run response.
//
//	@Description	Per-rule match + peek-driven would-allow result
type DryRunResponseJson struct {
	RequestLabelSnapshot map[string]string `json:"request_label_snapshot"`
	Matched              []interface{}     `json:"matched"`
	NotMatched           []interface{}     `json:"not_matched"`
}

type DryRunMatchJson struct {
	RateLimitId      string `json:"rate_limit_id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string `json:"namespace" example:"root.acme"`
	EffectiveMode    string `json:"effective_mode" example:"enforce"`
	BucketKey        string `json:"bucket_key" example:"actor=act_abc|labels/team=acme"`
	AlgorithmSummary string `json:"algorithm_summary" example:"token bucket 60 @ 1/s"`
	WouldAllow       bool   `json:"would_allow"`
	Remaining        int    `json:"remaining"`
	RetryAfterMs     int64  `json:"retry_after_ms"`
	PeekFailed       bool   `json:"peek_failed"`
}

type DryRunNotMatchedJson struct {
	RateLimitId string `json:"rate_limit_id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string `json:"namespace" example:"root.acme"`
	Reason      string `json:"reason"`
}
