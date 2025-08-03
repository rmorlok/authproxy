package request_log

import (
	"github.com/google/uuid"
	"net/http"
	"time"
)

// RoundTrip implements the http.RoundTripper interface
func (t *redisLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	// Generate a unique ID for this request
	id := uuid.New()

	// Record start time
	startTime := time.Now()

	// Execute the request
	resp, err := t.transport.RoundTrip(req)

	// Create a log entry
	entry := &Entry{
		ID:        id,
		Timestamp: startTime,
		Request: EntryRequest{
			URL:           req.URL.String(),
			HttpVersion:   req.Proto,
			Method:        req.Method,
			Headers:       req.Header,
			ContentLength: req.ContentLength,
		},
		Duration: time.Since(startTime),
	}

	// If there was an error, log it
	if err != nil {
		entry.Response.StatusCode = http.StatusInternalServerError
		entry.Response.Headers = make(map[string][]string)
		entry.Response.Err = err.Error()

		// Store the entry in Redis asynchronously
		go t.storeEntryInRedis(entry)

		return resp, err
	}

	// Copy response data
	entry.Response.HttpVersion = resp.Proto
	entry.Response.StatusCode = resp.StatusCode
	entry.Response.Headers = resp.Header
	entry.Response.ContentLength = resp.ContentLength

	// Store the entry in Redis asynchronously
	go t.storeEntryInRedis(entry)

	return resp, nil
}
