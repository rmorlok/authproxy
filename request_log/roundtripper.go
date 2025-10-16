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
	var requestBodyTrackingReader trackingReader
	var responseBodyTrackingReader trackingReader

	// Generate a unique ID for this request
	id := apctx.GetUuidGenerator(ctx).New()

	// Record start time
	clock := apctx.GetClock(ctx)
	startTime := clock.Now()

	if req.Body != nil {
		if t.recordFullRequest && t.maxFullRequestSize > 0 {
			// Create a buffer to store the request body
			requestBodyBuf = &bytes.Buffer{}

			// Create a TeeReader that copies to our buffer but passes through all data
			bodyReader := io.TeeReader(
				io.LimitReader(req.Body, int64(t.maxFullRequestSize)),
				requestBodyBuf,
			)

			// Split the reader so the data is read from the tee reader, but the close happens on the body
			split := newSplitReadCloser(bodyReader, req.Body)

			// Standardize the tracking of response size regardless of if we are tracking the body
			requestBodyTrackingReader = split

			// Replace the request body with our reader while preserving the original closer
			req.Body = split
		} else {
			// Track the body size
			tracking := newTrackingReadCloser(req.Body)

			// Standardize the tracking of response size regardless of if we are tracking the body
			requestBodyTrackingReader = tracking

			// Replace the body so it goes through our tracking
			req.Body = tracking
		}
	} else {
		requestBodyTrackingReader = &noOpTrackingReader{}
	}

	// Execute the request
	resp, err := t.transport.RoundTrip(req)

	reqContentLength := req.ContentLength
	if reqContentLength == -1 {
		reqContentLength = requestBodyTrackingReader.BytesRead()
	}

	// Create a log entry
	entry := &Entry{
		ID:            id,
		CorrelationID: apctx.CorrelationID(ctx),
		Timestamp:     startTime,
		Full:          t.recordFullRequest,
		Request: EntryRequest{
			URL:           req.URL.String(),
			HttpVersion:   req.Proto,
			Method:        req.Method,
			Headers:       req.Header,
			ContentLength: reqContentLength,
		},
		MillisecondDuration: MillisecondDuration(clock.Since(startTime)),
	}

	// If there was an error, log it
	if err != nil {
		entry.Response.StatusCode = http.StatusInternalServerError
		entry.Response.Err = err.Error()

		// Store the entry in Redis asynchronously
		go func() {
			if t.recordFullRequest {
				if requestBodyBuf != nil {
					requestData, err := io.ReadAll(requestBodyBuf)
					if err != nil {
						t.logger.Error("error reading full request body", "error", err, "entry_id", entry.ID.String())
						entry.Request.Body = []byte(err.Error())
					} else {
						entry.Request.Body = requestData
					}
				}

				// Because we are in an error case, the body cannot be assumed to be populated.
			}

			err := t.persistEntry(entry)
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
	entry.Response.ContentLength = resp.ContentLength // This will be overwritten if we are recording the full response

	if t.recordFullRequest && t.maxFullResponseSize > 0 && resp.Body != nil {
		var responseBodyWriter *io.PipeWriter

		// Create a new reader that allows us to read the response without consuming it
		responseBodyReader, responseBodyWriter = io.Pipe()
		originalBody := resp.Body

		// Create a TeeReader to copy the response to our writer while still passing it through
		teeReader := io.TeeReader(io.LimitReader(originalBody, int64(t.maxFullResponseSize)), responseBodyWriter)

		bodyReader := newSplitReadCloser(teeReader, originalBody, responseBodyWriter)

		// Track the response being read for size as well
		responseBodyTrackingReader = bodyReader

		// Replace the response body with our tee reader
		resp.Body = bodyReader

	} else {
		bodyReader := newTrackingReadCloser(resp.Body)

		// Track the response being read for size as well
		responseBodyTrackingReader = bodyReader

		// Replace the response body with our tee reader
		resp.Body = bodyReader
	}

	// Store the entry in Redis asynchronously
	go func() {
		if t.recordFullRequest {
			if requestBodyBuf != nil {
				requestData, err := io.ReadAll(requestBodyBuf)
				if err != nil {
					t.logger.Error("error reading full request body", "error", err, "entry_id", entry.ID.String())
					entry.Request.Body = []byte(err.Error())
				} else {
					entry.Request.Body = requestData
				}
			}

			if responseBodyReader != nil {
				responseData, err := io.ReadAll(responseBodyReader)
				if err != nil {
					t.logger.Error("error reading full request body", "error", err, "entry_id", entry.ID.String())
					entry.Response.Body = []byte(err.Error())
				} else {
					entry.Response.Body = responseData
				}
			}
		}

		// This select should immediately return the body reader done if the full request is being recorded. This
		// is to cover cases where we aren't recording the full response but need to wait for the client to fully
		// consume the data.
		select {
		case <-ctx.Done():
			entry.RequestCancelled = true
		case <-responseBodyTrackingReader.Done():
			if entry.Response.ContentLength <= 0 {
				entry.Response.ContentLength = responseBodyTrackingReader.BytesRead()
			}
		case <-time.After(t.maxResponseWait):
			entry.InternalTimeout = true
			t.logger.Error("timed out waiting for response body to be read; entry will not have accurate size", "entry_id", entry.ID.String(), "correlation_id", entry.CorrelationID, "max_wait", t.maxResponseWait.String())
		}

		err := t.persistEntry(entry)
		if err != nil {
			t.logger.Error("error storing HTTP log entry in Redis", "error", err, "entry_id", entry.ID.String(), "correlation_id", entry.CorrelationID)
		}
	}()

	return resp, nil
}
