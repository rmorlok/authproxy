package request_log

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
)

// LogRecord represents a record of an HTTP request as is stored in the request log. This
// data is redacted to avoid containing sensitive information like information in headers. For a given
// record, the full request may be stored as well, which would correspond to the data in the
// Entry struct.
//
// JSON tagging on this struct is used so the same data structure can be passed directly to endpoint
// responses. It is not use for internal storage.
type LogRecord struct {
	Namespace           string              `json:"namespace"`
	Type                httpf.RequestType   `json:"type"`
	RequestId           apid.ID             `json:"request_id"`
	CorrelationId       string              `json:"correlation_id,omitempty"`
	Timestamp           time.Time           `json:"timestamp"`
	MillisecondDuration MillisecondDuration `json:"duration"`
	ConnectionId        apid.ID             `json:"connection_id,omitempty"`
	ConnectorId         apid.ID             `json:"connector_id,omitempty"`
	ConnectorVersion    uint64              `json:"connector_version,omitempty"`
	Method              string              `json:"method"`
	Host                string              `json:"host"`
	Scheme              string              `json:"scheme"`
	Path                string              `json:"path"`
	RequestHttpVersion  string              `json:"request_http_version,omitempty"`
	RequestSizeBytes    int64               `json:"request_size_bytes,omitempty"`
	RequestMimeType     string              `json:"request_mime_type,omitempty"`
	// RequestBodySkipped explains why the request body was not captured
	// into the full log (chunked / unknown size, or larger than the
	// configured cap). Empty when captured. See BodySkippedReason.
	RequestBodySkipped  BodySkippedReason   `json:"request_body_skipped,omitempty"`
	ResponseStatusCode  int                 `json:"response_status_code,omitempty"`
	ResponseError       string              `json:"response_error,omitempty"`
	ResponseHttpVersion string              `json:"response_http_version,omitempty"`
	ResponseSizeBytes   int64               `json:"response_size_bytes,omitempty"`
	ResponseMimeType    string              `json:"response_mime_type,omitempty"`
	// ResponseBodySkipped mirrors RequestBodySkipped for the response
	// side — chunked SSE / LLM token streams are the common case.
	ResponseBodySkipped BodySkippedReason   `json:"response_body_skipped,omitempty"`
	InternalTimeout     bool                `json:"internal_timeout,omitempty"`
	RequestCancelled    bool                `json:"request_cancelled,omitempty"`
	FullRequestRecorded bool                `json:"full_request_recorded,omitempty"`
	Labels              database.Labels     `json:"labels,omitempty"`

	// ResponseSource identifies who produced the response. Defaults to
	// ResponseSourceUpstream so historical entries — and any non-429
	// response — keep the obvious meaning. See attribution.go.
	ResponseSource ResponseSource `json:"response_source,omitempty"`

	// RateLimitId, RateLimitMode, RateLimitBucket are populated when a
	// proxy-side RateLimit resource matched the request, regardless of
	// whether it was the firing rule or just a logged observation. The
	// connector-level reactive limiter does not populate these (it has
	// no rule id).
	RateLimitId     apid.ID           `json:"rate_limit_id,omitempty"`
	RateLimitMode   string            `json:"rate_limit_mode,omitempty"`
	RateLimitBucket map[string]string `json:"rate_limit_bucket,omitempty"`

	// RateLimitMatched is the full set of rate-limit rules that matched
	// this request — the firing rule plus any observe-mode matches. Lets
	// operators see *every* rule that contributed to the decision, not
	// just the one that ultimately rejected the request.
	RateLimitMatched []RateLimitMatch `json:"rate_limit_matched,omitempty"`
}

func (e *LogRecord) GetId() apid.ID {
	return e.RequestId
}

func (e *LogRecord) GetNamespace() string {
	return e.Namespace
}
