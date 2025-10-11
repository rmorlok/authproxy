package request_log

import (
	"net/url"
	"time"

	"github.com/google/uuid"
)

// This file contains Entry/EntryRequest/EntryResponse structs which represent the request
// as it was captured from the HTTP request. This value is serialized to JSON to store the
// full request. To represent the redacted version of the request, an EntryRecord is used,
// which is a subset of the EntryRequest and organized for searching.

type EntryRequest struct {
	URL           string              `json:"u"`
	HttpVersion   string              `json:"v"`
	Method        string              `json:"m"`
	Headers       map[string][]string `json:"h"`
	ContentLength int64               `json:"cl,omitempty"`
	Body          []byte              `json:"b,omitempty"`
}

func (e *EntryRequest) setRedisRecordFields(er *EntryRecord) {
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

type EntryResponse struct {
	HttpVersion   string              `json:"v"`
	StatusCode    int                 `json:"sc"`
	Headers       map[string][]string `json:"h"`
	Body          []byte              `json:"b,omitempty"`
	ContentLength int64               `json:"cl,omitempty"`
	Err           string              `json:"err,omitempty"`
}

func (e *EntryResponse) setRedisRecordFields(er *EntryRecord) {
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

type Entry struct {
	ID                  uuid.UUID           `json:"id"`
	CorrelationID       string              `json:"cid"`
	Timestamp           time.Time           `json:"ts"`
	MillisecondDuration MillisecondDuration `json:"dur"`
	InternalTimeout     bool                `json:"to,omitempty"`
	RequestCancelled    bool                `json:"rc,omitempty"`
	Request             EntryRequest        `json:"req"`
	Response            EntryResponse       `json:"res"`
}

func (e *Entry) setRedisRecordFields(er *EntryRecord) {
	if e == nil {
		return
	}

	er.RequestId = e.ID
	er.Timestamp = e.Timestamp
	er.MillisecondDuration = e.MillisecondDuration
	er.CorrelationId = e.CorrelationID

	e.Request.setRedisRecordFields(er)
	e.Response.setRedisRecordFields(er)
}
