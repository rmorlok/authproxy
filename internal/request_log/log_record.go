package request_log

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
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
	RequestId           apid.ID           `json:"request_id"`
	CorrelationId       string              `json:"correlation_id,omitempty"`
	Timestamp           time.Time           `json:"timestamp"`
	MillisecondDuration MillisecondDuration `json:"duration"`
	ConnectionId        apid.ID           `json:"connection_id,omitempty"`
	ConnectorId         apid.ID           `json:"connector_id,omitempty"`
	ConnectorVersion    uint64              `json:"connector_version,omitempty"`
	Method              string              `json:"method"`
	Host                string              `json:"host"`
	Scheme              string              `json:"scheme"`
	Path                string              `json:"path"`
	RequestHttpVersion  string              `json:"request_http_version,omitempty"`
	RequestSizeBytes    int64               `json:"request_size_bytes,omitempty"`
	RequestMimeType     string              `json:"request_mime_type,omitempty"`
	ResponseStatusCode  int                 `json:"response_status_code,omitempty"`
	ResponseError       string              `json:"response_error,omitempty"`
	ResponseHttpVersion string              `json:"response_http_version,omitempty"`
	ResponseSizeBytes   int64               `json:"response_size_bytes,omitempty"`
	ResponseMimeType    string              `json:"response_mime_type,omitempty"`
	InternalTimeout     bool                `json:"internal_timeout,omitempty"`
	RequestCancelled    bool                `json:"request_cancelled,omitempty"`
	FullRequestRecorded bool                `json:"full_request_recorded,omitempty"`
}

func (e *LogRecord) GetId() apid.ID {
	return e.RequestId
}

func (e *LogRecord) GetNamespace() string {
	return e.Namespace
}
