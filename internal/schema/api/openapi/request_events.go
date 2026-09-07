package openapi

import "github.com/rmorlok/authproxy/internal/schema/resources/meta"

type RequestEventNamespaceReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Namespace" example:"Namespace"`
	ID         string `json:"id" binding:"required" example:"root.acme"`
}

type RequestEventActorReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Actor" example:"Actor"`
	ID         string `json:"id" binding:"required" example:"act_01example"`
	Name       string `json:"name,omitempty" example:"billing-service"`
	Namespace  string `json:"namespace,omitempty" example:"root.acme"`
}

type RequestEventConnectionReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Connection" example:"Connection"`
	ID         string `json:"id" binding:"required" example:"cxn_01example"`
	Name       string `json:"name,omitempty" example:"production"`
	Namespace  string `json:"namespace,omitempty" example:"root.acme"`
}

type RequestEventConnectorReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"Connector" example:"Connector"`
	ID         string `json:"id" binding:"required" example:"cxr_01example"`
	Name       string `json:"name,omitempty" example:"provider"`
	Namespace  string `json:"namespace,omitempty" example:"root.integrations"`
	Generation uint64 `json:"generation,omitempty" example:"3"`
}

type RequestEventRateLimitReferenceJson struct {
	APIVersion string `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string `json:"kind" binding:"required" enums:"RateLimit" example:"RateLimit"`
	ID         string `json:"id" binding:"required" example:"rl_01example"`
}

type RequestEventRequestJson struct {
	Method      string `json:"method" binding:"required" example:"GET"`
	Host        string `json:"host" binding:"required" example:"api.example.com"`
	Scheme      string `json:"scheme" binding:"required" example:"https"`
	Path        string `json:"path" binding:"required" example:"/v1/users"`
	HTTPVersion string `json:"httpVersion,omitempty" example:"HTTP/2.0"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	MIMEType    string `json:"mimeType,omitempty" example:"application/json"`
	BodySkipped string `json:"bodySkipped,omitempty" enums:"streaming,too_large"`
}

type RequestEventResponseJson struct {
	StatusCode  int    `json:"statusCode,omitempty" example:"200"`
	Error       string `json:"error,omitempty"`
	HTTPVersion string `json:"httpVersion,omitempty" example:"HTTP/2.0"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	MIMEType    string `json:"mimeType,omitempty" example:"application/json"`
	BodySkipped string `json:"bodySkipped,omitempty" enums:"streaming,too_large"`
	Source      string `json:"source,omitempty" enums:"upstream,connector_rate_limiter,rate_limit" example:"upstream"`
}

type RequestEventCapturedRequestJson struct {
	URL     string              `json:"url" binding:"required" example:"https://api.example.com/v1/users"`
	Headers map[string][]string `json:"headers" binding:"required"`
	Body    []byte              `json:"body,omitempty" swaggertype:"string" format:"byte"`
}

type RequestEventCapturedResponseJson struct {
	Headers map[string][]string `json:"headers" binding:"required"`
	Body    []byte              `json:"body,omitempty" swaggertype:"string" format:"byte"`
}

type RequestEventCaptureJson struct {
	Request  RequestEventCapturedRequestJson  `json:"request" binding:"required"`
	Response RequestEventCapturedResponseJson `json:"response" binding:"required"`
}

type RequestEventRateLimitJson struct {
	RateLimitRef RequestEventRateLimitReferenceJson `json:"rateLimitRef" binding:"required"`
	Mode         string                             `json:"mode" binding:"required" enums:"enforce,observe" example:"enforce"`
	Bucket       map[string]string                  `json:"bucket,omitempty"`
}

type RequestEventSpecJson struct {
	RequestType          string                               `json:"requestType" binding:"required" enums:"global,proxy,oauth,public,probe" example:"proxy"`
	CorrelationID        string                               `json:"correlationId,omitempty"`
	DurationMilliseconds int64                                `json:"durationMilliseconds" binding:"required" example:"150"`
	NamespaceRef         RequestEventNamespaceReferenceJson   `json:"namespaceRef" binding:"required"`
	ActorRef             *RequestEventActorReferenceJson      `json:"actorRef,omitempty"`
	ConnectionRef        *RequestEventConnectionReferenceJson `json:"connectionRef,omitempty"`
	ConnectorRef         *RequestEventConnectorReferenceJson  `json:"connectorRef,omitempty"`
	Request              RequestEventRequestJson              `json:"request" binding:"required"`
	Response             RequestEventResponseJson             `json:"response" binding:"required"`
	CaptureAvailable     bool                                 `json:"captureAvailable"`
	Capture              *RequestEventCaptureJson             `json:"capture,omitempty"`
	RateLimit            *RequestEventRateLimitJson           `json:"rateLimit,omitempty"`
	RateLimitsMatched    []RequestEventRateLimitJson          `json:"rateLimitsMatched,omitempty"`
	InternalTimeout      bool                                 `json:"internalTimeout,omitempty"`
	RequestCancelled     bool                                 `json:"requestCancelled,omitempty"`
}

// RequestEventJson documents an immutable, resource-shaped event projection.
//
//	@Description	Immutable, resource-shaped HTTP request event projection
type RequestEventJson struct {
	APIVersion string               `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string               `json:"kind" binding:"required" enums:"RequestEvent" example:"RequestEvent"`
	Metadata   meta.ObjectMeta      `json:"metadata" binding:"required"`
	Spec       RequestEventSpecJson `json:"spec" binding:"required"`
}

type RequestEventListMetaJson struct {
	Continue string `json:"continue,omitempty"`
	Total    *int64 `json:"total,omitempty"`
}

// ListRequestEventsResponseJson documents the request-event projection list.
//
//	@Description	Paginated list of immutable request-event projections
type ListRequestEventsResponseJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"RequestEventList" example:"RequestEventList"`
	Metadata   RequestEventListMetaJson `json:"metadata" binding:"required"`
	Items      []RequestEventJson       `json:"items" binding:"required"`
}
