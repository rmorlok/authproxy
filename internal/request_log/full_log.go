package request_log

import (
	"net/url"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
)

// This file contains Entry/EntryRequest/EntryResponse structs which represent the request
// as it was captured from the HTTP request. This value is serialized to JSON to store the
// full request. To represent the redacted version of the request, an LogRecord is used,
// which is a subset of the EntryRequest and organized for searching.

type FullLogRequest struct {
	URL           string              `json:"u"`
	HttpVersion   string              `json:"v"`
	Method        string              `json:"m"`
	Headers       map[string][]string `json:"h"`
	ContentLength int64               `json:"cl,omitempty"`
	Body          []byte              `json:"b,omitempty"`
}

func (e *FullLogRequest) setRecordFields(er *LogRecord) {
	if e == nil {
		return
	}

	if parsedURL, err := url.Parse(e.URL); err == nil {
		er.Scheme = parsedURL.Scheme
		er.Host = parsedURL.Host
		er.Path = parsedURL.Path
	}

	er.Method = e.Method
	er.RequestHttpVersion = e.HttpVersion
	er.RequestSizeBytes = e.ContentLength

	if e.Headers != nil && e.Headers["Content-Type"] != nil && len(e.Headers["Content-Type"]) > 0 {
		er.RequestMimeType = e.Headers["Content-Type"][0]
	}
}

type FullLogResponse struct {
	HttpVersion   string              `json:"v"`
	StatusCode    int                 `json:"sc"`
	Headers       map[string][]string `json:"h"`
	Body          []byte              `json:"b,omitempty"`
	ContentLength int64               `json:"cl,omitempty"`
	Err           string              `json:"err,omitempty"`
}

func (e *FullLogResponse) setRecordFields(er *LogRecord) {
	if e == nil {
		return
	}

	er.ResponseHttpVersion = e.HttpVersion

	if e.Headers != nil && e.Headers["Content-Type"] != nil && len(e.Headers["Content-Type"]) > 0 {
		er.ResponseMimeType = e.Headers["Content-Type"][0]
	}

	er.ResponseSizeBytes = e.ContentLength
	er.ResponseStatusCode = e.StatusCode
	er.ResponseError = e.Err
}

type FullLog struct {
	Id                  apid.ID             `json:"id"`
	Namespace           string              `json:"ns"`
	CorrelationID       string              `json:"cid"`
	Timestamp           time.Time           `json:"ts"`
	MillisecondDuration MillisecondDuration `json:"dur"`
	Full                bool                `json:"full,omitempty"`
	InternalTimeout     bool                `json:"to,omitempty"`
	RequestCancelled    bool                `json:"rc,omitempty"`
	Request             FullLogRequest      `json:"req"`
	Response            FullLogResponse     `json:"res"`
}

func (e *FullLog) GetId() apid.ID {
	return e.Id
}

func (e *FullLog) GetNamespace() string {
	return e.Namespace
}

func (e *FullLog) setRecordFields(er *LogRecord) {
	if e == nil {
		return
	}

	er.RequestId = e.Id
	er.Namespace = e.Namespace
	er.Timestamp = e.Timestamp
	er.MillisecondDuration = e.MillisecondDuration
	er.CorrelationId = e.CorrelationID
	er.FullRequestRecorded = e.Full

	e.Request.setRecordFields(er)
	e.Response.setRecordFields(er)
}

func (e *FullLog) ToRecord() *LogRecord {
	er := &LogRecord{}
	e.setRecordFields(er)
	return er
}

func NewFullLogFromRecord(er *LogRecord) *FullLog {
	if er == nil {
		return nil
	}

	entry := &FullLog{
		Id:                  er.RequestId,
		Namespace:           er.Namespace,
		CorrelationID:       er.CorrelationId,
		Timestamp:           er.Timestamp,
		MillisecondDuration: er.MillisecondDuration,
		InternalTimeout:     er.InternalTimeout,
		RequestCancelled:    er.RequestCancelled,
		Full:                false,
	}

	// Construct URL from components
	url := url.URL{
		Scheme: er.Scheme,
		Host:   er.Host,
		Path:   er.Path,
	}

	// Populate Request
	entry.Request = FullLogRequest{
		URL:           url.String(),
		HttpVersion:   er.RequestHttpVersion,
		Method:        er.Method,
		ContentLength: er.RequestSizeBytes,
		Headers:       make(map[string][]string),
	}
	if er.RequestMimeType != "" {
		entry.Request.Headers["Content-Type"] = []string{er.RequestMimeType}
	}

	// Populate Response
	entry.Response = FullLogResponse{
		HttpVersion:   er.ResponseHttpVersion,
		StatusCode:    er.ResponseStatusCode,
		ContentLength: er.ResponseSizeBytes,
		Err:           er.ResponseError,
		Headers:       make(map[string][]string),
	}
	if er.ResponseMimeType != "" {
		entry.Response.Headers["Content-Type"] = []string{er.ResponseMimeType}
	}

	return entry
}
