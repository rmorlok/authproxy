package app_metrics

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
)

// LogRecord represents a record of an HTTP request as is stored in the request events. This
// data is redacted to avoid containing sensitive information like information in headers. For a given
// record, the full request may be stored as well, which would correspond to the data in the
// Entry struct.
//
// JSON tagging on this struct is used so the same data structure can be passed directly to endpoint
// responses. It is not use for internal storage.
type LogRecord struct {
	Namespace           string              `json:"namespace"`
	Type                httpf.RequestType   `json:"type"`
	RequestId           apid.ID             `json:"requestId"`
	CorrelationId       string              `json:"correlationId,omitempty"`
	Timestamp           time.Time           `json:"timestamp"`
	MillisecondDuration MillisecondDuration `json:"duration"`
	ConnectionId        apid.ID             `json:"connectionId,omitempty"`
	ConnectorId         apid.ID             `json:"connectorId,omitempty"`
	ConnectorVersion    uint64              `json:"connectorVersion,omitempty"`
	Method              string              `json:"method"`
	Host                string              `json:"host"`
	Scheme              string              `json:"scheme"`
	Path                string              `json:"path"`
	RequestHttpVersion  string              `json:"requestHttpVersion,omitempty"`
	RequestSizeBytes    int64               `json:"requestSizeBytes,omitempty"`
	RequestMimeType     string              `json:"requestMimeType,omitempty"`
	// RequestBodySkipped explains why the request body was not captured
	// into the full log (chunked / unknown size, or larger than the
	// configured cap). Empty when captured. See BodySkippedReason.
	RequestBodySkipped  BodySkippedReason `json:"requestBodySkipped,omitempty"`
	ResponseStatusCode  int               `json:"responseStatusCode,omitempty"`
	ResponseError       string            `json:"responseError,omitempty"`
	ResponseHttpVersion string            `json:"responseHttpVersion,omitempty"`
	ResponseSizeBytes   int64             `json:"responseSizeBytes,omitempty"`
	ResponseMimeType    string            `json:"responseMimeType,omitempty"`
	// ResponseBodySkipped mirrors RequestBodySkipped for the response
	// side — chunked SSE / LLM token streams are the common case.
	ResponseBodySkipped BodySkippedReason `json:"responseBodySkipped,omitempty"`
	InternalTimeout     bool              `json:"internalTimeout,omitempty"`
	RequestCancelled    bool              `json:"requestCancelled,omitempty"`
	FullRequestRecorded bool              `json:"fullRequestRecorded,omitempty"`
	Labels              database.Labels   `json:"labels,omitempty"`

	// ResponseSource identifies who produced the response. Defaults to
	// ResponseSourceUpstream so historical entries — and any non-429
	// response — keep the obvious meaning. See attribution.go.
	ResponseSource ResponseSource `json:"responseSource,omitempty"`

	// RateLimitId, RateLimitMode, RateLimitBucket are populated when a
	// proxy-side RateLimit resource matched the request, regardless of
	// whether it was the firing rule or just a logged observation. The
	// connector-level reactive limiter does not populate these (it has
	// no rule id).
	RateLimitId     apid.ID           `json:"rateLimitId,omitempty"`
	RateLimitMode   string            `json:"rateLimitMode,omitempty"`
	RateLimitBucket map[string]string `json:"rateLimitBucket,omitempty"`

	// RateLimitMatched is the full set of rate-limit rules that matched
	// this request — the firing rule plus any observe-mode matches. Lets
	// operators see *every* rule that contributed to the decision, not
	// just the one that ultimately rejected the request.
	RateLimitMatched []RateLimitMatch `json:"rateLimitMatched,omitempty"`
}

func (e *LogRecord) GetId() apid.ID {
	return e.RequestId
}

func (e *LogRecord) GetNamespace() string {
	return e.Namespace
}
