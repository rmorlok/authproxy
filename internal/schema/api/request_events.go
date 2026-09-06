package api

import (
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

const RequestEventKind meta.Kind = "RequestEvent"

// RequestEventJson is an immutable, resource-shaped observation of one HTTP
// exchange. It is an event projection, not a client-managed desired resource.
// Captured protocol data remains in Spec.Capture and is always subject to the
// API secret-replay policy.
//
//	@Description	Immutable, resource-shaped HTTP request event projection
type RequestEventJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      meta.ObjectMeta      `json:"metadata" yaml:"metadata"`
	Spec          RequestEventSpecJson `json:"spec" yaml:"spec"`
}

type RequestEventSpecJson struct {
	RequestType          common.RequestType          `json:"requestType" yaml:"requestType"`
	CorrelationID        string                      `json:"correlationId,omitempty" yaml:"correlationId,omitempty"`
	DurationMilliseconds int64                       `json:"durationMilliseconds" yaml:"durationMilliseconds"`
	NamespaceRef         meta.ObjectReference        `json:"namespaceRef" yaml:"namespaceRef"`
	ActorRef             *meta.ObjectReference       `json:"actorRef,omitempty" yaml:"actorRef,omitempty"`
	ConnectionRef        *meta.ObjectReference       `json:"connectionRef,omitempty" yaml:"connectionRef,omitempty"`
	ConnectorRef         *meta.ObjectReference       `json:"connectorRef,omitempty" yaml:"connectorRef,omitempty"`
	Request              RequestEventRequestJson     `json:"request" yaml:"request"`
	Response             RequestEventResponseJson    `json:"response" yaml:"response"`
	CaptureAvailable     bool                        `json:"captureAvailable" yaml:"captureAvailable"`
	Capture              *RequestEventCaptureJson    `json:"capture,omitempty" yaml:"capture,omitempty"`
	RateLimit            *RequestEventRateLimitJson  `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
	RateLimitsMatched    []RequestEventRateLimitJson `json:"rateLimitsMatched,omitempty" yaml:"rateLimitsMatched,omitempty"`
	InternalTimeout      bool                        `json:"internalTimeout,omitempty" yaml:"internalTimeout,omitempty"`
	RequestCancelled     bool                        `json:"requestCancelled,omitempty" yaml:"requestCancelled,omitempty"`
}

type RequestEventRequestJson struct {
	Method      string `json:"method" yaml:"method"`
	Host        string `json:"host" yaml:"host"`
	Scheme      string `json:"scheme" yaml:"scheme"`
	Path        string `json:"path" yaml:"path"`
	HTTPVersion string `json:"httpVersion,omitempty" yaml:"httpVersion,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty" yaml:"sizeBytes,omitempty"`
	MIMEType    string `json:"mimeType,omitempty" yaml:"mimeType,omitempty"`
	BodySkipped string `json:"bodySkipped,omitempty" yaml:"bodySkipped,omitempty"`
}

type RequestEventResponseJson struct {
	StatusCode  int    `json:"statusCode,omitempty" yaml:"statusCode,omitempty"`
	Error       string `json:"error,omitempty" yaml:"error,omitempty"`
	HTTPVersion string `json:"httpVersion,omitempty" yaml:"httpVersion,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty" yaml:"sizeBytes,omitempty"`
	MIMEType    string `json:"mimeType,omitempty" yaml:"mimeType,omitempty"`
	BodySkipped string `json:"bodySkipped,omitempty" yaml:"bodySkipped,omitempty"`
	Source      string `json:"source,omitempty" yaml:"source,omitempty"`
}

// RequestEventCaptureJson contains the optional encrypted full-log material.
// Its sensitive members are redacted unless the caller has secrets/replay.
type RequestEventCaptureJson struct {
	Request  RequestEventCapturedRequestJson  `json:"request" yaml:"request"`
	Response RequestEventCapturedResponseJson `json:"response" yaml:"response"`
}

type RequestEventCapturedRequestJson struct {
	URL     string              `json:"url" yaml:"url" apiredact:"secret"`
	Headers map[string][]string `json:"headers" yaml:"headers" apiredact:"secret"`
	// Body is the captured body encoded as base64 for the JSON/YAML wire format.
	Body string `json:"body,omitempty" yaml:"body,omitempty" apiredact:"secret"`
}

type RequestEventCapturedResponseJson struct {
	Headers map[string][]string `json:"headers" yaml:"headers" apiredact:"secret"`
	// Body is the captured body encoded as base64 for the JSON/YAML wire format.
	Body string `json:"body,omitempty" yaml:"body,omitempty" apiredact:"secret"`
}

type RequestEventRateLimitJson struct {
	RateLimitRef meta.ObjectReference `json:"rateLimitRef" yaml:"rateLimitRef"`
	Mode         string               `json:"mode" yaml:"mode"`
	Bucket       map[string]string    `json:"bucket,omitempty" yaml:"bucket,omitempty"`
}

// RequestEventListMetaJson keeps the existing exact total while moving the
// opaque cursor into Kubernetes-style list metadata.
type RequestEventListMetaJson struct {
	Continue string `json:"continue,omitempty" yaml:"continue,omitempty"`
	Total    *int64 `json:"total,omitempty" yaml:"total,omitempty"`
}

type ListRequestEventsResponseJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      RequestEventListMetaJson `json:"metadata" yaml:"metadata"`
	Items         []RequestEventJson       `json:"items" yaml:"items"`
}

func NewListRequestEventsResponseJson(
	items []RequestEventJson,
	continueToken string,
	total *int64,
) ListRequestEventsResponseJson {
	if items == nil {
		items = make([]RequestEventJson, 0)
	}
	return ListRequestEventsResponseJson{
		TypeMeta: meta.NewTypeMeta(meta.Kind(string(RequestEventKind) + "List")),
		Metadata: RequestEventListMetaJson{Continue: continueToken, Total: total},
		Items:    items,
	}
}
