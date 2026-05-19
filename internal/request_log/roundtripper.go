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
	// Install an Attribution on the request context so inner middlewares
	// (e.g. the connector-level reactive 429 limiter) can stamp who
	// produced the response. We share the pointer with this round-tripper
	// so we read the same struct on the way back out — see attribution.go.
	if AttributionFromContext(ctx) == nil {
		ctx = ContextWithAttribution(ctx, &Attribution{})
		req = req.WithContext(ctx)
	}
	// Create a context that won't be cancelled when the request completes, for use in async
	// goroutines that store logs after the round trip returns. This preserves context values
	// (correlation ID, clock, etc.) but prevents "context canceled" errors.
	asyncCtx := context.WithoutCancel(ctx)
	cc := t.captureConfig
	var responseBodyReader *io.PipeReader
	var requestBodyBuf *bytes.Buffer
	var requestBodyTrackingReader trackingReader
	var responseBodyTrackingReader trackingReader
	var requestBodySkipped BodySkippedReason
	var responseBodySkipped BodySkippedReason

	// Generate a unique ID for this request
	id := apctx.GetIdGenerator(ctx).New(apid.PrefixRequestLog)

	// Record start time
	clock := apctx.GetClock(ctx)
	startTime := clock.Now()

	if req.Body != nil {
		if cc.recordFullRequest && cc.maxFullRequestSize > 0 {
			// Size-bounded capture: decide tee-vs-skip up front based on
			// the advance-known Content-Length. Streaming bodies (chunked,
			// no Content-Length) and bodies larger than the configured cap
			// are forwarded *un-tee'd* so the upstream sees the full
			// stream and the proxy does not accumulate an unbounded body
			// in memory. The "skipped" reason lands on the log record so
			// operators can see why a body wasn't captured.
			switch {
			case req.ContentLength < 0:
				requestBodySkipped = BodySkippedStreaming
				tracking := newTrackingReadCloser(req.Body)
				requestBodyTrackingReader = tracking
				req.Body = tracking
			case uint64(req.ContentLength) > cc.maxFullRequestSize:
				requestBodySkipped = BodySkippedTooLarge
				tracking := newTrackingReadCloser(req.Body)
				requestBodyTrackingReader = tracking
				req.Body = tracking
			default:
				// ContentLength is set and within the cap. Tee straight
				// through — no LimitReader, since we know the body fits.
				requestBodyBuf = &bytes.Buffer{}
				bodyReader := io.TeeReader(req.Body, requestBodyBuf)
				split := newSplitReadCloser(bodyReader, req.Body)
				requestBodyTrackingReader = split
				req.Body = split
			}
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
			BodySkipped:   requestBodySkipped,
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
			ApplyAttributionToLogRecord(record, ctx)

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
		// Same size-bounded decision as the request side, against the
		// upstream's Content-Length and max_full_response_size. SSE /
		// chunked streams skip the tee — the whole point of the raw
		// path is "don't buffer the upstream response."
		switch {
		case resp.ContentLength < 0:
			responseBodySkipped = BodySkippedStreaming
			bodyReader := newTrackingReadCloser(resp.Body)
			responseBodyTrackingReader = bodyReader
			resp.Body = bodyReader
		case uint64(resp.ContentLength) > cc.maxFullResponseSize:
			responseBodySkipped = BodySkippedTooLarge
			bodyReader := newTrackingReadCloser(resp.Body)
			responseBodyTrackingReader = bodyReader
			resp.Body = bodyReader
		default:
			var responseBodyWriter *io.PipeWriter

			// Create a new reader that allows us to read the response without consuming it
			responseBodyReader, responseBodyWriter = io.Pipe()
			originalBody := resp.Body

			// Content-Length is known and within the cap — tee without
			// a LimitReader so the upstream body passes through whole.
			teeReader := io.TeeReader(originalBody, responseBodyWriter)

			bodyReader := newSplitReadCloser(teeReader, originalBody, responseBodyWriter)

			// Track the response being read for size as well
			responseBodyTrackingReader = bodyReader

			// Replace the response body with our tee reader
			resp.Body = bodyReader
		}
	} else {
		bodyReader := newTrackingReadCloser(resp.Body)

		// Track the response being read for size as well
		responseBodyTrackingReader = bodyReader

		// Replace the response body with our tee reader
		resp.Body = bodyReader
	}
	full_log.Response.BodySkipped = responseBodySkipped

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
		ApplyAttributionToLogRecord(record, ctx)

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
