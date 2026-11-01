package app_metrics

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

// RoundTripper records request/response metadata and optionally captures bodies
// without delaying response headers while a request upload is still in flight.
type RoundTripper struct {
	store         RecordStore
	fullStore     FullStore
	requestInfo   httpf.RequestInfo
	captureConfig captureConfig
	transport     http.RoundTripper
	logger        *slog.Logger
}

// RoundTrip returns upstream headers as soon as the transport supplies them.
// Body capture and final byte counts are persisted asynchronously after reads
// settle, including when response headers arrive before an upload finishes.
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
	id := apctx.GetIdGenerator(ctx).New(apid.PrefixRequestEvents)

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
			ContentLength: req.ContentLength,
			BodySkipped:   requestBodySkipped,
		},
		MillisecondDuration: MillisecondDuration(clock.Since(startTime)),
	}

	// If there was an error, log it
	if err != nil {
		requestErr := err
		full_log.Response.StatusCode = http.StatusInternalServerError
		full_log.Response.Err = requestErr.Error()

		// Store the full_log in Redis asynchronously
		go func() {
			t.finishRequestCapture(full_log, requestBodyTrackingReader, requestBodyBuf)

			record := full_log.ToRecord()
			SetLogRecordFieldsFromRequestInfo(record, t.requestInfo)
			ApplyAttributionToLogRecord(record, ctx)

			if storeErr := t.store.StoreRecord(asyncCtx, record); storeErr != nil {
				t.logger.Error(
					"error storing HTTP log record",
					"error", storeErr,
					"entry_id", full_log.Id.String(),
					"correlation_id", full_log.CorrelationID,
				)
				return
			}

			if t.fullStore != nil {
				if storeErr := t.fullStore.Store(asyncCtx, full_log); storeErr != nil {
					t.logger.Error(
						"error storing full HTTP log in blob storage",
						"error", storeErr,
						"entry_id", full_log.Id.String(),
					)
				}
			}
		}()

		return resp, requestErr
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
			// Drain the response tee before waiting for upload completion. A
			// full-duplex caller may need these bytes before sending more data.
			if responseBodyReader != nil {
				responseData, err := io.ReadAll(responseBodyReader)
				if err != nil {
					t.logger.Error(
						"error reading full request body",
						"error", err,
						"entry_id", full_log.Id.String(),
					)
					full_log.Response.Body = []byte(err.Error())
				} else {
					full_log.Response.Body = responseData
				}
			}
		}

		// This select should immediately return the body reader done if the full
		// request is being recorded. This is to cover cases where we aren't
		// recording the full response but need to wait for the client to fully
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
			t.logger.Error(
				"timed out waiting for response body to be read; full_log will not have accurate size",
				"entry_id", full_log.Id.String(),
				"correlation_id", full_log.CorrelationID,
				"max_wait", cc.maxResponseWait.String(),
			)
		}
		t.finishRequestCapture(full_log, requestBodyTrackingReader, requestBodyBuf)

		record := full_log.ToRecord()
		SetLogRecordFieldsFromRequestInfo(record, t.requestInfo)
		ApplyAttributionToLogRecord(record, ctx)

		if err := t.store.StoreRecord(asyncCtx, record); err != nil {
			t.logger.Error(
				"error storing HTTP log record",
				"error", err,
				"entry_id", full_log.Id.String(),
				"correlation_id", full_log.CorrelationID,
			)
		}

		if t.fullStore != nil {
			if err := t.fullStore.Store(asyncCtx, full_log); err != nil {
				t.logger.Error(
					"error storing full HTTP log entry in blob storage",
					"error", err,
					"record_id", full_log.Id.String(),
				)
			}
		}
	}()

	return resp, nil
}

// finishRequestCapture runs only in the asynchronous logging worker. A transport
// may return headers or an error before its request-body reader has stopped, so
// the capture buffer is inspected only after Done provides a read/close barrier.
// Waiting is bounded, and never blocks response-pipe draining or header return.
func (t *RoundTripper) finishRequestCapture(
	log *FullLog,
	tracking trackingReader,
	body *bytes.Buffer,
) {
	if body == nil && log.Request.ContentLength >= 0 {
		return
	}

	// Request cancellation can race the transport's asynchronous body Close
	// after a successful exchange. Wait for that required cleanup rather than
	// discard a complete capture just because the handler's context ended.
	completed := false
	select {
	case <-tracking.Done():
		completed = true
	default:
		timer := time.NewTimer(t.captureConfig.maxResponseWait)
		defer timer.Stop()
		select {
		case <-tracking.Done():
			completed = true
		case <-timer.C:
			log.InternalTimeout = true
			t.logger.Error(
				"timed out waiting for request body to close; full_log will not have accurate size or body",
				"entry_id", log.Id.String(),
				"max_wait", t.captureConfig.maxResponseWait.String(),
			)
		}
	}

	if log.Request.ContentLength < 0 {
		// On timeout this remains a safe, monotonic partial count; the log's
		// InternalTimeout flag explains why completion could not be observed.
		log.Request.ContentLength = tracking.BytesRead()
	}
	if completed && body != nil {
		// bytes.Buffer cannot fail reads; its tee writer is now quiescent.
		log.Request.Body, _ = io.ReadAll(body)
	}
}
