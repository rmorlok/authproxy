package request_log

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type RoundTripper struct {
	store         RecordStore
	fullStore     FullStore
	requestInfo   httpf.RequestInfo
	captureConfig captureConfig
	transport     http.RoundTripper
	logger        *slog.Logger
}

// RoundTrip implements the http.RoundTripper interface
func (t *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	// Create a context that won't be cancelled when the request completes, for use in async
	// goroutines that store logs after the round trip returns. This preserves context values
	// (correlation ID, clock, etc.) but prevents "context canceled" errors.
	asyncCtx := context.WithoutCancel(ctx)
	cc := t.captureConfig
	var responseBodyReader *io.PipeReader
	var requestBodyBuf *bytes.Buffer
	var requestBodyTrackingReader trackingReader
	var responseBodyTrackingReader trackingReader

	// Generate a unique ID for this request
	id := apctx.GetIdGenerator(ctx).New(apid.PrefixRequestLog)

	// Record start time
	clock := apctx.GetClock(ctx)
	startTime := clock.Now()

	if req.Body != nil {
		if cc.recordFullRequest && cc.maxFullRequestSize > 0 {
			// Create a buffer to store the request body
			requestBodyBuf = &bytes.Buffer{}

			// Create a TeeReader that copies to our buffer but passes through all data
			bodyReader := io.TeeReader(
				io.LimitReader(req.Body, int64(cc.maxFullRequestSize)),
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

	// Create a log full_log
	full_log := &FullLog{
		Id:            id,
		Namespace:     t.requestInfo.Namespace,
		CorrelationID: apctx.CorrelationID(ctx), // TODO: this won't populate because ctx is local
		Timestamp:     startTime,
		Full:          cc.recordFullRequest,
		Request: FullLogRequest{
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
		full_log.Response.StatusCode = http.StatusInternalServerError
		full_log.Response.Err = err.Error()

		// Store the full_log in Redis asynchronously
		go func() {
			if cc.recordFullRequest {
				if requestBodyBuf != nil {
					requestData, err := io.ReadAll(requestBodyBuf)
					if err != nil {
						t.logger.Error("error reading full request body", "error", err, "entry_id", full_log.Id.String())
						full_log.Request.Body = []byte(err.Error())
					} else {
						full_log.Request.Body = requestData
					}
				}

				// Because we are in an error case, the body cannot be assumed to be populated.
			}

			record := full_log.ToRecord()
			SetLogRecordFieldsFromRequestInfo(record, t.requestInfo)

			err = t.store.StoreRecord(asyncCtx, record)
			if err != nil {
				t.logger.Error("error storing HTTP log record", "error", err, "entry_id", full_log.Id.String(), "correlation_id", full_log.CorrelationID)
				return
			}

			if t.fullStore != nil {
				err = t.fullStore.Store(asyncCtx, full_log)
				if err != nil {
					t.logger.Error("error storing full HTTP log in blob storage", "error", err, "entry_id", full_log.Id.String())
				}
			}
		}()

		return resp, err
	}

	// Copy response data
	full_log.Response.HttpVersion = resp.Proto
	full_log.Response.StatusCode = resp.StatusCode
	full_log.Response.Headers = resp.Header
	full_log.Response.ContentLength = resp.ContentLength // This will be overwritten if we are recording the full response

	if cc.recordFullRequest && cc.maxFullResponseSize > 0 && resp.Body != nil {
		var responseBodyWriter *io.PipeWriter

		// Create a new reader that allows us to read the response without consuming it
		responseBodyReader, responseBodyWriter = io.Pipe()
		originalBody := resp.Body

		// Create a TeeReader to copy the response to our writer while still passing it through
		teeReader := io.TeeReader(io.LimitReader(originalBody, int64(cc.maxFullResponseSize)), responseBodyWriter)

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

	// Store the full_log in Redis asynchronously
	go func() {
		if cc.recordFullRequest {
			if requestBodyBuf != nil {
				requestData, err := io.ReadAll(requestBodyBuf)
				if err != nil {
					t.logger.Error("error reading full request body", "error", err, "entry_id", full_log.Id.String())
					full_log.Request.Body = []byte(err.Error())
				} else {
					full_log.Request.Body = requestData
				}
			}

			if responseBodyReader != nil {
				responseData, err := io.ReadAll(responseBodyReader)
				if err != nil {
					t.logger.Error("error reading full request body", "error", err, "entry_id", full_log.Id.String())
					full_log.Response.Body = []byte(err.Error())
				} else {
					full_log.Response.Body = responseData
				}
			}
		}

		// This select should immediately return the body reader done if the full request is being recorded. This
		// is to cover cases where we aren't recording the full response but need to wait for the client to fully
		// consume the data.
		select {
		case <-ctx.Done():
			full_log.RequestCancelled = true
		case <-responseBodyTrackingReader.Done():
			if full_log.Response.ContentLength <= 0 {
				full_log.Response.ContentLength = responseBodyTrackingReader.BytesRead()
			}
		case <-time.After(cc.maxResponseWait):
			full_log.InternalTimeout = true
			t.logger.Error("timed out waiting for response body to be read; full_log will not have accurate size", "entry_id", full_log.Id.String(), "correlation_id", full_log.CorrelationID, "max_wait", cc.maxResponseWait.String())
		}

		record := full_log.ToRecord()
		SetLogRecordFieldsFromRequestInfo(record, t.requestInfo)

		if err := t.store.StoreRecord(asyncCtx, record); err != nil {
			t.logger.Error("error storing HTTP log record", "error", err, "entry_id", full_log.Id.String(), "correlation_id", full_log.CorrelationID)
		}

		if t.fullStore != nil {
			if err := t.fullStore.Store(asyncCtx, full_log); err != nil {
				t.logger.Error("error storing full HTTP log entry in blob storage", "error", err, "record_id", full_log.Id.String())
			}
		}
	}()

	return resp, nil
}
