package openapi

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

// ResourceListJson is the non-generic OpenAPI projection of the common list
// transport. Concrete list adapters embed it and specialize Items because
// swaggo cannot reliably document instantiated generic Go types.
type ResourceListJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required"`
	Metadata   apiv1alpha1.ListMeta `json:"metadata" binding:"required"`
}

// ListActorsResponseJson documents the paginated actor list response.
//
//	@Description	Paginated list of actors
type ListActorsResponseJson struct {
	ResourceListJson
	// List of actors.
	Items []schemaapi.ActorJson `json:"items" binding:"required"`
}

// ListNamespacesResponseJson documents the paginated namespace list response.
//
//	@Description	Paginated list of namespaces
type ListNamespacesResponseJson struct {
	ResourceListJson
	// List of namespaces.
	Items []nschema.Namespace `json:"items" binding:"required"`
}

// ListConnectorsResponseJson documents the paginated connector list response.
//
//	@Description	Paginated list of connectors
type ListConnectorsResponseJson struct {
	ResourceListJson
	// List of connectors.
	Items []schemaapi.ConnectorJson `json:"items" binding:"required"`
}

// ConnectorVersionJson documents a connector version response.
//
//	@Description	Detailed connector version information
type ConnectorVersionJson struct {
	Id          apid.ID                         `json:"id" swaggertype:"string" example:"cxr_test550e8400abcde"`
	Version     uint64                          `json:"version" example:"1"`
	Namespace   string                          `json:"namespace" example:"root.acme"`
	Name        string                          `json:"name" example:"salesforce"`
	State       schemaapi.ConnectorVersionState `json:"state" swaggertype:"string" example:"primary"`
	Definition  interface{}                     `json:"definition"`
	Labels      map[string]string               `json:"labels,omitempty"`
	Annotations map[string]string               `json:"annotations,omitempty"`
	CreatedAt   time.Time                       `json:"createdAt"`
	UpdatedAt   time.Time                       `json:"updatedAt"`
}

// ListConnectorVersionsResponseJson documents the paginated connector version list response.
//
//	@Description	Paginated list of connector versions
type ListConnectorVersionsResponseJson struct {
	ResourceListJson
	// List of connector versions.
	Items []ConnectorVersionJson `json:"items" binding:"required"`
}

// ConnectionJson documents a connection response while keeping nested
// connector/setup definitions opaque for swaggo.
//
//	@Description	Connection to an external service
type ConnectionJson struct {
	Id          apid.ID           `json:"id" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Namespace   string            `json:"namespace" example:"root.acme"`
	Name        string            `json:"name" example:"production-crm"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	State       string            `json:"state" example:"configured"`
	HealthState string            `json:"healthState" example:"healthy"`
	SetupStep   string            `json:"setupStepId,omitempty" example:"tenant"`
	SetupError  string            `json:"setupError,omitempty"`
	Connector   interface{}       `json:"connector"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// ListConnectionResponseJson documents the paginated connection list response.
//
//	@Description	Paginated list of connections
type ListConnectionResponseJson struct {
	ResourceListJson
	Items []ConnectionJson `json:"items" binding:"required"`
}

// DisconnectResponseJson documents the connection disconnect response.
//
//	@Description	Response for disconnect operation
type DisconnectResponseJson struct {
	TaskId     string      `json:"taskId"`
	Connection interface{} `json:"connection"`
}

// DisconnectConnectionRequestJson documents connection disconnect operation bodies.
//
//	@Description	Request body for connection disconnect operations
type DisconnectConnectionRequestJson struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" example:"600"`
}

// MigrateConnectionVersionRequestJson documents connection version migration operation bodies.
//
//	@Description	Request body for connection connector-version migration operations
type MigrateConnectionVersionRequestJson struct {
	TargetVersion  uint64 `json:"targetVersion" example:"3"`
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" example:"600"`
}

// MigrateConnectionVersionResponseJson documents the connection version migration response.
//
//	@Description	Response for connection connector-version migration operation
type MigrateConnectionVersionResponseJson struct {
	TaskId        string  `json:"taskId"`
	ConnectionId  apid.ID `json:"connectionId" swaggertype:"string" example:"cxn_test550e8400abcde"`
	SourceVersion uint64  `json:"sourceVersion" example:"1"`
	TargetVersion uint64  `json:"targetVersion" example:"3"`
}

// NotificationJson documents actor-visible notifications.
//
//	@Description	Actor-visible notification
type NotificationJson struct {
	Id           apid.ID                `json:"id" swaggertype:"string" example:"ntf_test550e8400abcde"`
	Key          string                 `json:"key"`
	Level        string                 `json:"level" example:"warning"`
	State        string                 `json:"state" example:"active"`
	ResourceType string                 `json:"resourceType" example:"connection"`
	ResourceId   apid.ID                `json:"resourceId" swaggertype:"string" example:"cxn_test550e8400abcde"`
	Namespace    string                 `json:"namespace" example:"root.acme"`
	Title        string                 `json:"title"`
	Message      string                 `json:"message"`
	ActionUrl    string                 `json:"actionUrl,omitempty"`
	CanAction    bool                   `json:"canAction"`
	Viewed       bool                   `json:"viewed"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
	ResolvedAt   *time.Time             `json:"resolvedAt,omitempty"`
}

// ListNotificationsResponseJson documents the paginated notification list response.
//
//	@Description	Paginated list of actor-visible notifications
type ListNotificationsResponseJson struct {
	Items  []interface{} `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
}

// CreateConnectorRequestJson documents the connector creation body.
//
//	@Description	Request to create a new connector
type CreateConnectorRequestJson struct {
	Namespace   string            `json:"namespace" example:"root.acme"`
	Name        *string           `json:"name,omitempty" example:"salesforce"`
	Definition  interface{}       `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// UpdateConnectorRequestJson documents the connector update body.
//
//	@Description	Request to update a logical connector
type UpdateConnectorRequestJson struct {
	Name        *string            `json:"name,omitempty" example:"salesforce"`
	Definition  interface{}        `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// UpdateConnectorVersionRequestJson documents connector definition-version updates.
//
//	@Description Request to update a connector definition version
type UpdateConnectorVersionRequestJson struct {
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

// ConnectorLifecycleRequestJson documents connector lifecycle operation bodies.
//
//	@Description	Request to run a connector lifecycle operation
type ConnectorLifecycleRequestJson struct {
	TimeoutSeconds *int64 `json:"timeoutSeconds,omitempty" example:"600"`
}

// ConnectorLifecycleResponseJson documents connector lifecycle operation responses.
//
//	@Description	Response for connector lifecycle operation
type ConnectorLifecycleResponseJson struct {
	TaskId      string  `json:"taskId"`
	ConnectorId apid.ID `json:"connectorId" swaggertype:"string" example:"cxr_test550e8400abcde"`
}

// KeySpecJson documents managed-key desired state while keeping polymorphic
// provider configuration opaque to swaggo. No key-material examples belong in
// this projection.
type KeySpecJson struct {
	Usage        keyschema.KeyUsage        `json:"usage,omitempty" enums:"data_encryption"`
	MaterialType keyschema.KeyMaterialType `json:"materialType,omitempty" enums:"symmetric,public,private,external"`
	DesiredState keyschema.KeyState        `json:"desiredState,omitempty" enums:"active,disabled"`
	KeyData      map[string]interface{}    `json:"keyData,omitempty" swaggertype:"object"`
}

// KeyJson documents a managed key resource response.
//
//	@Description	Kubernetes-style managed Key resource. Secret keyData fields are always redacted in responses.
type KeyJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required" enums:"Key" example:"Key"`
	Metadata   meta.ObjectMeta      `json:"metadata" binding:"required"`
	Spec       KeySpecJson          `json:"spec" binding:"required"`
	Status     *keyschema.KeyStatus `json:"status,omitempty"`
}

// ListKeysResponseJson documents the paginated key list response.
//
//	@Description	Paginated list of keys
type ListKeysResponseJson struct {
	ResourceListJson
	Items []KeyJson `json:"items" binding:"required"`
}

// ListRequestEventsResponseJson documents the paginated request-events list response.
//
//	@Description	Paginated list of request events entries
type ListRequestEventsResponseJson struct {
	Items  []interface{} `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
	Total  *int64        `json:"total,omitempty"`
}

// MetricsSchemaResponseJson documents the metrics schema response.
//
//	@Description	Application metrics schema response
type MetricsSchemaResponseJson struct {
	Metrics []schemaapi.MetricsSchemaMetricJson `json:"metrics"`
}

// RequestEventJson documents the public request-event record projection.
//
//	@Description	HTTP request events entry
type RequestEventJson struct {
	Namespace           string            `json:"namespace" example:"root.acme"`
	Type                string            `json:"type" example:"proxy"`
	RequestId           string            `json:"requestId" swaggertype:"string" example:"req_test550e8400abcde"`
	CorrelationId       string            `json:"correlationId,omitempty"`
	Timestamp           time.Time         `json:"timestamp"`
	MillisecondDuration int64             `json:"duration" example:"150"`
	ConnectionId        string            `json:"connectionId,omitempty" swaggertype:"string"`
	ConnectorId         string            `json:"connectorId,omitempty" swaggertype:"string"`
	ConnectorVersion    uint64            `json:"connectorVersion,omitempty"`
	Method              string            `json:"method" example:"GET"`
	Host                string            `json:"host" example:"api.example.com"`
	Scheme              string            `json:"scheme" example:"https"`
	Path                string            `json:"path" example:"/v1/users"`
	ResponseStatusCode  int               `json:"responseStatusCode,omitempty" example:"200"`
	Labels              map[string]string `json:"labels,omitempty"`
	ResponseSource      string            `json:"responseSource,omitempty" example:"upstream"`
	RateLimitId         string            `json:"rateLimitId,omitempty" swaggertype:"string"`
	RateLimitMode       string            `json:"rateLimitMode,omitempty"`
	RateLimitBucket     map[string]string `json:"rateLimitBucket,omitempty"`
	RateLimitMatched    []interface{}     `json:"rateLimitMatched,omitempty"`
}

// TaskInfoJson documents public background task status.
//
//	@Description	Background task status
type TaskInfoJson struct {
	Id        string `json:"id"`
	Type      string `json:"type"`
	State     string `json:"state" example:"completed"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// KeySpecPatchJson documents mutable desired key fields.
type KeySpecPatchJson struct {
	Usage        *keyschema.KeyUsage        `json:"usage,omitempty" enums:"data_encryption"`
	MaterialType *keyschema.KeyMaterialType `json:"materialType,omitempty" enums:"symmetric,public,private,external"`
	DesiredState *keyschema.KeyState        `json:"desiredState,omitempty" enums:"active,disabled"`
	KeyData      map[string]interface{}     `json:"keyData,omitempty" swaggertype:"object"`
}

// KeyPatchJson documents the managed-key update body.
//
//	@Description	Kubernetes-style managed Key patch. Status and server-owned metadata are rejected.
type KeyPatchJson struct {
	APIVersion string                `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                `json:"kind" binding:"required" enums:"Key" example:"Key"`
	Metadata   *meta.ObjectMetaPatch `json:"metadata" binding:"required"`
	Spec       *KeySpecPatchJson     `json:"spec" binding:"required"`
	Status     *keyschema.KeyStatus  `json:"status,omitempty"`
}

// RateLimitJson documents a rate-limit response while keeping the definition
// opaque for swaggo.
//
//	@Description	Rate-limit API response
type RateLimitJson struct {
	Id          apid.ID           `json:"id" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string            `json:"namespace" example:"root.acme"`
	Name        string            `json:"name" example:"public-api"`
	Definition  map[string]any    `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// ListRateLimitsResponseJson documents the paginated rate-limit list response.
//
//	@Description	Paginated list of rate limits
type ListRateLimitsResponseJson struct {
	ResourceListJson
	Items []RateLimitJson `json:"items" binding:"required"`
}

// CreateRateLimitRequestJson documents the rate-limit creation body.
//
//	@Description	Request to create a rate limit
type CreateRateLimitRequestJson struct {
	Namespace   string            `json:"namespace" example:"root.acme"`
	Name        *string           `json:"name,omitempty" example:"public-api"`
	Definition  map[string]any    `json:"definition"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// UpdateRateLimitRequestJson documents the rate-limit update body.
//
//	@Description	Request to update a rate limit
type UpdateRateLimitRequestJson struct {
	Name        *string            `json:"name,omitempty" example:"public-api"`
	Definition  map[string]any     `json:"definition,omitempty"`
	Labels      *map[string]string `json:"labels,omitempty"`
	Annotations *map[string]string `json:"annotations,omitempty"`
}

// ProxyRequestJson documents the proxy-shaped request used by dry-run.
//
//	@Description	Request to proxy or simulate an HTTP request
type ProxyRequestJson struct {
	URL      string            `json:"url" example:"https://api.example.com/v1/users"`
	Method   string            `json:"method" example:"POST"`
	Headers  map[string]any    `json:"headers,omitempty" swaggertype:"object"`
	Labels   map[string]string `json:"labels,omitempty"`
	BodyRaw  []byte            `json:"bodyRaw,omitempty"`
	BodyJson interface{}       `json:"bodyJson,omitempty"`
}

// DryRunRequestJson documents the rate-limit dry-run request body.
//
//	@Description	Dry-run input: a proxy-shaped request + request type + the identity it runs under
type DryRunRequestJson struct {
	Request     interface{} `json:"request"`
	RequestType string      `json:"requestType" example:"proxy"`
	Context     interface{} `json:"context"`
}

// DryRunContextJson documents the dry-run identity context.
//
//	@Description	Identity the request runs under
type DryRunContextJson struct {
	ConnectionId string `json:"connectionId,omitempty"`
	ActorId      string `json:"actorId,omitempty"`
	Namespace    string `json:"namespace,omitempty" example:"root.acme"`
}

// DryRunResponseJson documents the dry-run response.
//
//	@Description	Per-rule match + peek-driven would-allow result
type DryRunResponseJson struct {
	RequestLabelSnapshot map[string]string `json:"requestLabelSnapshot"`
	Matched              []interface{}     `json:"matched"`
	NotMatched           []interface{}     `json:"notMatched"`
}

type DryRunMatchJson struct {
	RateLimitId      string `json:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace        string `json:"namespace" example:"root.acme"`
	EffectiveMode    string `json:"effectiveMode" example:"enforce"`
	BucketKey        string `json:"bucketKey" example:"actor=act_abc|labels/team=acme"`
	AlgorithmSummary string `json:"algorithmSummary" example:"token bucket 60 @ 1/s"`
	WouldAllow       bool   `json:"wouldAllow"`
	Remaining        int    `json:"remaining"`
	RetryAfterMs     int64  `json:"retryAfterMs"`
	PeekFailed       bool   `json:"peekFailed"`
}

type DryRunNotMatchedJson struct {
	RateLimitId string `json:"rateLimitId" swaggertype:"string" example:"rl_test550e8400abcde"`
	Namespace   string `json:"namespace" example:"root.acme"`
	Reason      string `json:"reason"`
}

// ProxyResponseJson documents the response from a proxied request.
//
//	@Description	Response from a proxied HTTP request
type ProxyResponseJson struct {
	StatusCode int               `json:"statusCode" example:"200"`
	Headers    map[string]string `json:"headers,omitempty"`
	BodyRaw    []byte            `json:"bodyRaw,omitempty"`
	BodyJson   interface{}       `json:"bodyJson,omitempty"`
}
