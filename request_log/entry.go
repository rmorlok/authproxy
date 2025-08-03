package request_log

import (
	"github.com/google/uuid"
	"net/url"
	"time"
)

type EntryRequest struct {
	URL           string              `json:"u"`
	HttpVersion   string              `json:"v"`
	Method        string              `json:"m"`
	Headers       map[string][]string `json:"h"`
	ContentLength int64               `json:"cl,omitempty"`
	Body          []byte              `json:"b,omitempty"`
}

func (e *EntryRequest) setRedisRecordFields(vals map[string]interface{}) {
	if e == nil {
		return
	}

	if parsedURL, err := url.Parse(e.URL); err == nil {
		vals[fieldScheme] = parsedURL.Scheme
		vals[fieldHost] = parsedURL.Host
		vals[fieldPath] = parsedURL.Path
	}

	vals[fieldMethod] = e.Method
	vals[fieldRequestHttpVersion] = e.HttpVersion
	vals[fieldRequestSizeBytes] = e.ContentLength

	if e.Headers != nil && e.Headers["Content-Type"] != nil && len(e.Headers["Content-Type"]) > 0 {
		vals[fieldRequestMimeTypes] = e.Headers["Content-Type"][0]
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

func (e *EntryResponse) setRedisRecordFields(vals map[string]interface{}) {
	if e == nil {
		return
	}

	if e.HttpVersion != "" {
		vals[fieldResponseHttpVersion] = e.HttpVersion
	}

	if e.Headers != nil && e.Headers["Content-Type"] != nil && len(e.Headers["Content-Type"]) > 0 {
		vals[fieldResponseMimeTypes] = e.Headers["Content-Type"][0]
	}

	vals[fieldResponseSizeBytes] = e.ContentLength

	if e.StatusCode != 0 {
		vals[fieldResponseStatusCode] = e.StatusCode
	}

	if e.Err != "" {
		vals[fieldResponseError] = e.Err
	}
}

type Entry struct {
	ID            uuid.UUID     `json:"id"`
	CorrelationID string        `json:"cid"`
	Timestamp     time.Time     `json:"ts"`
	Duration      time.Duration `json:"dur"`
	Request       EntryRequest  `json:"req"`
	Response      EntryResponse `json:"res"`
}

func (e *Entry) setRedisRecordFields(vals map[string]interface{}) {
	if e == nil {
		return
	}

	vals[fieldRequestId] = e.ID.String()
	vals[fieldTimestamp] = e.Timestamp
	vals[fieldDurationMs] = e.Duration

	if e.CorrelationID != "" {
		vals[fieldCorrelationId] = e.CorrelationID
	}

	e.Request.setRedisRecordFields(vals)
	e.Response.setRedisRecordFields(vals)
}
