package request_log

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/apctx"
)

// RoundTrip implements the http.RoundTripper interface
func (t *redisLogger) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	var responseBodyReader *io.PipeReader
	var requestBodyBuf *bytes.Buffer

	// Generate a unique ID for this request
	id := apctx.GetUuidGenerator(ctx).New()

	// Record start time
	startTime := apctx.GetClock(ctx).Now()

	if t.recordFullRequest && t.maxFullRequestSize > 0 && req.Body != nil {
		// Create a buffer to store the request body
		requestBodyBuf = &bytes.Buffer{}

		// Create a TeeReader that copies to our buffer but passes through all data
		bodyReader := io.TeeReader(
			io.LimitReader(req.Body, int64(t.maxFullRequestSize)),
			requestBodyBuf,
		)

		// Replace the request body with our reader while preserving the original closer
		req.Body = &bodyReadCloser{
			reader: bodyReader,
			closer: req.Body,
		}
	}

	// Execute the request
	resp, err := t.transport.RoundTrip(req)

	// Create a log entry
	entry := &Entry{
		ID:            id,
		CorrelationID: apctx.CorrelationID(ctx),
		Timestamp:     startTime,
		Request: EntryRequest{
			URL:           req.URL.String(),
			HttpVersion:   req.Proto,
			Method:        req.Method,
			Headers:       req.Header,
			ContentLength: req.ContentLength,
		},
		MillisecondDuration: MillisecondDuration(time.Since(startTime)),
	}

	// If there was an error, log it
	if err != nil {
		entry.Response.StatusCode = http.StatusInternalServerError
		entry.Response.Err = err.Error()

		// Store the entry in Redis asynchronously
		go func() {
			err := t.persistEntry(entry, requestBodyBuf, responseBodyReader)
			if err != nil {
				t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String(), "correlation_id", entry.CorrelationID)
			}
		}()

		return resp, err
	}

	// Copy response data
	entry.Response.HttpVersion = resp.Proto
	entry.Response.StatusCode = resp.StatusCode
	entry.Response.Headers = resp.Header
	entry.Response.ContentLength = resp.ContentLength

	if t.recordFullRequest && t.maxFullResponseSize > 0 && resp.Body != nil {
		var responseBodyWriter *io.PipeWriter

		// Create a new reader that allows us to read the response without consuming it
		responseBodyReader, responseBodyWriter = io.Pipe()
		originalBody := resp.Body

		// Create a TeeReader to copy the response to our writer while still passing it through
		teeReader := io.TeeReader(io.LimitReader(originalBody, int64(t.maxFullResponseSize)), responseBodyWriter)

		// Replace the response body with our tee reader
		resp.Body = &wrappedReadCloser{
			Reader:     teeReader,
			closer:     originalBody,
			pipeWriter: responseBodyWriter,
		}

	}

	// Store the entry in Redis asynchronously
	go func() {
		err := t.persistEntry(entry, requestBodyBuf, responseBodyReader)
		if err != nil {
			t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String(), "correlation_id", entry.CorrelationID)
		}
	}()

	return resp, nil
}
