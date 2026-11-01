//go:build integration

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rawEvidenceHeader is spelled independently from the server constant so the
// integration suite checks the public wire contract rather than its definition.
const rawEvidenceHeader = "X-AuthProxy-Send-Evidence"

// rawEvidenceResult retains headers and trailers separately, plus a body read
// error for truncated streams that have already committed a successful status.
type rawEvidenceResult struct {
	Status  int
	Header  http.Header
	Trailer http.Header
	Body    []byte
	ReadErr error
}

// rawEvidenceRequest creates a real signed raw POST with a recognizable body.
// Requests are prepared before concurrent sends to avoid unrelated auth setup.
func rawEvidenceRequest(t *testing.T, env *helpers.IntegrationTestEnv, connection, target string) *http.Request {
	t.Helper()
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(http.MethodPost,
		env.ServerURL+"/api/v1/connections/"+connection+"/_proxyRaw", strings.NewReader("recorded raw operation"),
		sconfig.RootNamespace, "raw-evidence-actor", aschema.AllPermissions())
	require.NoError(t, err)
	req.Header.Set("X-AuthProxy-Upstream-URL", target)
	return req
}

// doRawEvidence consumes a live response while retaining stream-read failures.
// It has no testing.T calls so concurrent tests can assert in their owner goroutine.
func doRawEvidence(req *http.Request) (rawEvidenceResult, error) {
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return rawEvidenceResult{}, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	return rawEvidenceResult{resp.StatusCode, resp.Header.Clone(), resp.Trailer.Clone(), body, readErr}, nil
}

// assertRawEvidence checks the authoritative header independently of whether
// the status/body came from AuthProxy or the provider.
func assertRawEvidence(t *testing.T, req *http.Request, status int, evidence string) rawEvidenceResult {
	t.Helper()
	resp, err := doRawEvidence(req)
	require.NoError(t, err)
	require.NoError(t, resp.ReadErr)
	require.Equal(t, status, resp.Status, "body: %s", resp.Body)
	assert.Equal(t, []string{evidence}, resp.Header.Values(rawEvidenceHeader))
	return resp
}

// assertRawEvidenceError checks raw AuthProxy errors keep their previous JSON
// envelope: evidence is exclusively a response header on this endpoint.
func assertRawEvidenceError(t *testing.T, resp rawEvidenceResult) {
	t.Helper()
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	assert.NotEmpty(t, body["error"])
	assert.NotContains(t, body, "sendEvidence")
}

// TestRawProxySendEvidence_UpstreamResponses retains upstream status and bytes
// while confirming sends for successful and unsuccessful HTTP responses alike.
func TestRawProxySendEvidence_UpstreamResponses(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, status := range []int{200, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			path := fmt.Sprintf("/status/%d", status)
			upstream.Configure(path, helpers.ControlledResponse{
				Status: status, Headers: http.Header{"X-Upstream-Trace": {"retained"}}, Body: "provider bytes",
			})

			// Raw responses carry the provider's status directly and preserve bytes.
			resp := assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL(path)), status, "sent")
			assert.Equal(t, "provider bytes", string(resp.Body))
			assert.Equal(t, "retained", resp.Header.Get("X-Upstream-Trace"))
			recorded := upstream.Requests(path)
			require.Len(t, recorded, 1)
			assert.Equal(t, http.MethodPost, recorded[0].Method)
			assert.Equal(t, "recorded raw operation", recorded[0].Body)
		})
	}
}

// TestRawProxySendEvidence_EmptyResponses verifies response commitment does
// not depend on a body write for HEAD requests or a provider's 204 response.
func TestRawProxySendEvidence_EmptyResponses(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, scenario := range []struct {
		method string
		status int
	}{{http.MethodPost, 204}, {http.MethodHead, 200}, {http.MethodHead, 204}} {
		t.Run(fmt.Sprintf("%s/%d", scenario.method, scenario.status), func(t *testing.T) {
			path := fmt.Sprintf("/empty/%s/%d", scenario.method, scenario.status)
			upstream.Configure(path, helpers.ControlledResponse{Status: scenario.status})
			req := rawEvidenceRequest(t, env, connection, upstream.URL(path))
			req.Method = scenario.method
			if scenario.method == http.MethodHead {
				_ = req.Body.Close()
				req.Body, req.GetBody, req.ContentLength = nil, nil, 0
			}

			// Headers alone establish sent, with the upstream method/status intact.
			resp := assertRawEvidence(t, req, scenario.status, "sent")
			assert.Empty(t, resp.Body)
			recorded := upstream.Requests(path)
			require.Len(t, recorded, 1)
			assert.Equal(t, scenario.method, recorded[0].Method)
		})
	}
}

// TestRawProxySendEvidence_MalformedJSON keeps upstream bytes opaque even when
// the content type claims JSON that the wrapped endpoint would fail to decode.
func TestRawProxySendEvidence_MalformedJSON(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/malformed", helpers.ControlledResponse{
		Headers: http.Header{"Content-Type": {"application/json"}}, Body: "{invalid JSON",
	})

	// Raw forwarding must not turn a JSON decoding concern into an API error.
	resp := assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/malformed")), 200, "sent")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, "{invalid JSON", string(resp.Body))
	assert.Equal(t, 1, upstream.Count("/malformed"))
}

// TestRawProxySendEvidence_LocalRejections verifies the header exists before
// auth and route validation, without changing ordinary raw error JSON bodies.
func TestRawProxySendEvidence_LocalRejections(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, scenario := range []struct {
		name   string
		status int
		mutate func(*http.Request)
	}{
		{"missing-auth", 401, func(r *http.Request) { r.Header.Del("Authorization") }},
		{"invalid-auth", 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer invalid") }},
		{"missing-url", 400, func(r *http.Request) { r.Header.Del("X-AuthProxy-Upstream-URL") }},
		{"invalid-url", 400, func(r *http.Request) { r.Header.Set("X-AuthProxy-Upstream-URL", "ftp://unused.test") }},
		{"invalid-label", 400, func(r *http.Request) { r.Header.Set("X-AuthProxy-Label", "missing-separator") }},
		{"invalid-connection", 400, func(r *http.Request) { r.URL.Path = "/api/v1/connections/invalid/_proxyRaw" }},
		{"missing-connection", 404, func(r *http.Request) {
			r.URL.Path = "/api/v1/connections/" + apid.New(apid.PrefixConnection).String() + "/_proxyRaw"
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			req := rawEvidenceRequest(t, env, connection, upstream.URL("/never"))
			req.Header.Set(rawEvidenceHeader, "sent")
			scenario.mutate(req)

			// Caller evidence claims cannot change a known pre-send rejection.
			resp := assertRawEvidence(t, req, scenario.status, "notSent")
			assertRawEvidenceError(t, resp)
			assert.Zero(t, upstream.Count(""))
		})
	}
	t.Run("missing-permission", func(t *testing.T) {
		req := rawEvidenceRequest(t, env, connection, upstream.URL("/never"))
		token, err := env.ApiAuthUtil.GenerateBearerToken(req.Context(), "raw-read-only-actor", sconfig.RootNamespace,
			[]aschema.Permission{{Namespace: "root.**", Resources: []string{"connections"}, Verbs: []string{"get"}}})
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		// A valid identity lacking the proxy verb receives the same unsent header.
		resp := assertRawEvidence(t, req, 403, "notSent")
		assertRawEvidenceError(t, resp)
		assert.Zero(t, upstream.Count(""))
	})
}

// TestRawProxySendEvidence_RateLimit separates provider and locally generated
// 429s using the real Redis-backed backoff and an independent request count.
func TestRawProxySendEvidence_RateLimit(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, true)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/limited", helpers.ControlledResponse{
		Status: 429, Headers: http.Header{"Retry-After": {"60"}}, Body: "provider limited",
	})

	// A provider response starts the backoff and confirms the first send.
	assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/limited")), 429, "sent")
	require.Equal(t, 1, upstream.Count(""))
	upstream.Configure("/limited", helpers.ControlledResponse{Status: 200, Body: "allowed"})

	// The next identical status is synthesized without another upstream call.
	blocked := assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/limited")), 429, "notSent")
	assert.Equal(t, "true", blocked.Header.Get("X-Authproxy-Ratelimited"))
	assert.Equal(t, 1, upstream.Count(""))
}

// TestRawProxySendEvidence_TransportFailures distinguishes losing all response
// headers from losing a body after response evidence is already committed.
func TestRawProxySendEvidence_TransportFailures(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/drop", helpers.ControlledResponse{DropBeforeHeaders: true})

	// The upstream records a complete POST, then closes before any HTTP response.
	dropped := assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/drop")), 500, "mayHaveSent")
	assertRawEvidenceError(t, dropped)
	recorded := upstream.Requests("/drop")
	require.Len(t, recorded, 1)
	assert.Equal(t, "recorded raw operation", recorded[0].Body)

	// After a real response, a short body cannot revise the committed header.
	upstream.Configure("/truncated", helpers.ControlledResponse{Body: "partial", TruncateAfterHeaders: true, FlushHeaders: true})
	truncated, err := doRawEvidence(rawEvidenceRequest(t, env, connection, upstream.URL("/truncated")))
	require.NoError(t, err)
	assert.Equal(t, 200, truncated.Status)
	assert.Equal(t, []string{"sent"}, truncated.Header.Values(rawEvidenceHeader))
	assert.Equal(t, "partial", string(truncated.Body))
	require.ErrorIs(t, truncated.ReadErr, io.ErrUnexpectedEOF)
	assert.Equal(t, 1, upstream.Count("/truncated"))

	// Closing the redirect connection makes the redirected GET's drop
	// deterministic instead of allowing a transparent retry on a reused socket.
	upstream.Configure("/redirect", helpers.ControlledResponse{
		Status: 302, Headers: http.Header{"Location": {upstream.URL("/redirect-drop")}, "Connection": {"close"}},
	})
	upstream.Configure("/redirect-drop", helpers.ControlledResponse{DropBeforeHeaders: true})
	redirected := assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/redirect")), 500, "sent")
	assertRawEvidenceError(t, redirected)
	assert.Equal(t, 1, upstream.Count("/redirect"))
	assert.Equal(t, 1, upstream.Count("/redirect-drop"))
}

// TestRawProxySendEvidence_RedirectThenLocalBlock retains the initial redirect
// response when a separately established backoff suppresses the following hop.
func TestRawProxySendEvidence_RedirectThenLocalBlock(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, true)
	upstream := helpers.NewControlledUpstream(t)
	release, arrived := make(chan struct{}), make(chan struct{}, 1)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	upstream.Configure("/redirect", helpers.ControlledResponse{
		Status: 302, Headers: http.Header{"Location": {upstream.URL("/blocked-hop")}},
		Arrived: arrived, WaitFor: release,
	})
	upstream.Configure("/prime-backoff", helpers.ControlledResponse{
		Status: 429, Headers: http.Header{"Retry-After": {"60"}}, Body: "provider limited",
	})
	req := rawEvidenceRequest(t, env, connection, upstream.URL("/redirect"))
	type outcome struct {
		response rawEvidenceResult
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		resp, err := doRawEvidence(req)
		done <- outcome{resp, err}
	}()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("raw redirect request did not arrive")
	}

	// Prime actual rate limiting while the first request waits upstream, then
	// release its redirect so the next hop is rejected before target handoff.
	assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/prime-backoff")), 429, "sent")
	unblock()
	result := <-done
	require.NoError(t, result.err)
	require.NoError(t, result.response.ReadErr)
	assert.Equal(t, 429, result.response.Status)
	assert.Equal(t, []string{"sent"}, result.response.Header.Values(rawEvidenceHeader))
	assert.Equal(t, "true", result.response.Header.Get("X-Authproxy-Ratelimited"))
	assert.Equal(t, 1, upstream.Count("/redirect"))
	assert.Equal(t, 1, upstream.Count("/prime-backoff"))
	assert.Zero(t, upstream.Count("/blocked-hop"))
}

// TestRawProxySendEvidence_HeaderAndTrailerSpoofing checks both declared and
// undeclared trailer claims while preserving unrelated upstream metadata.
func TestRawProxySendEvidence_HeaderAndTrailerSpoofing(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("late=%v", late), func(t *testing.T) {
			path := fmt.Sprintf("/spoof/%v", late)
			action := helpers.ControlledResponse{
				Headers: http.Header{
					"X-Authproxy-Send-Evidence": {"notSent", "mayHaveSent"},
					"X-Upstream-Trace":          {"preserved"},
				},
				Body: "unchanged body", FlushHeaders: true,
			}
			trailers := http.Header{
				"X-Authproxy-Send-Evidence": {"notSent", "forged"},
				"X-Upstream-Final":          {"complete"},
			}
			if late {
				action.LateTrailers = trailers
			} else {
				action.Trailers = trailers
			}
			upstream.Configure(path, action)
			req := rawEvidenceRequest(t, env, connection, upstream.URL(path))
			req.Header.Add(rawEvidenceHeader, "notSent")
			req.Header.Add(rawEvidenceHeader, "forged")
			req.Header.Set("X-Caller-Trace", "retained")

			// A single authoritative header survives all provider-controlled
			// header/trailer forms; normal metadata and body still pass through.
			resp := assertRawEvidence(t, req, 200, "sent")
			assert.NotContains(t, resp.Trailer, http.CanonicalHeaderKey(rawEvidenceHeader),
				"the reserved name must not survive as a declaration or a trailer value")
			assert.Equal(t, "complete", resp.Trailer.Get("X-Upstream-Final"))
			assert.Equal(t, "preserved", resp.Header.Get("X-Upstream-Trace"))
			assert.Equal(t, "unchanged body", string(resp.Body))
			recorded := upstream.Requests(path)
			require.Len(t, recorded, 1)
			assert.Empty(t, recorded[0].Header.Values(rawEvidenceHeader))
			assert.Equal(t, "retained", recorded[0].Header.Get("X-Caller-Trace"))
		})
	}
}

// TestRawProxySendEvidence_SSEHeadersBeforeBody proves evidence is available
// while the stream remains open and before the upstream sends its first event.
func TestRawProxySendEvidence_SSEHeadersBeforeBody(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	upstream.Configure("/sse", helpers.ControlledResponse{
		Headers: http.Header{"Content-Type": {"text/event-stream"}}, Body: "data: first\n\n",
		FlushHeaders: true, WaitForBody: release,
	})
	req := rawEvidenceRequest(t, env, connection, upstream.URL("/sse"))
	type outcome struct {
		response *http.Response
		err      error
	}
	done := make(chan outcome, 1)
	client := &http.Client{Timeout: 10 * time.Second}
	go func() {
		resp, err := client.Do(req)
		done <- outcome{resp, err}
	}()

	// Client.Do must return from headers alone while the first body byte is gated.
	select {
	case result := <-done:
		require.NoError(t, result.err)
		defer result.response.Body.Close()
		assert.Equal(t, 200, result.response.StatusCode)
		assert.Equal(t, []string{"sent"}, result.response.Header.Values(rawEvidenceHeader))
		assert.Equal(t, "text/event-stream", result.response.Header.Get("Content-Type"))
		assert.Equal(t, 1, upstream.Count("/sse"))
		unblock()
		body, err := io.ReadAll(result.response.Body)
		require.NoError(t, err)
		assert.Equal(t, "data: first\n\n", string(body))
	case <-time.After(5 * time.Second):
		t.Fatal("raw response headers were buffered behind the first SSE body byte")
	}
}

// TestRawProxySendEvidence_ResponseBeforeUploadCompletes verifies committing
// evidence headers does not drain or close an upload that is still in flight.
// The 64 KiB body is below net/http's automatic request-drain threshold, where
// flushing without full-duplex support would otherwise wait for the gated tail.
func TestRawProxySendEvidence_ResponseBeforeUploadCompletes(t *testing.T) {
	for _, chunked := range []bool{false, true} {
		name := "known length"
		if chunked {
			name = "chunked"
		}
		t.Run(name, func(t *testing.T) {
			assertRawEvidenceResponseBeforeUploadCompletes(t, chunked)
		})
	}
}

// assertRawEvidenceResponseBeforeUploadCompletes exercises a gated upload with
// either framing mode and checks both the live exchange and eventual capture.
func assertRawEvidenceResponseBeforeUploadCompletes(t *testing.T, chunked bool) {
	t.Helper()
	env, connection := newWrappedEvidenceEnv(t, false)
	prefix := bytes.Repeat([]byte("p"), 16<<10)
	suffix := bytes.Repeat([]byte("s"), 48<<10)
	expectedUpload := append(append([]byte(nil), prefix...), suffix...)
	const responseBody = "complete upstream response"
	type upstreamResult struct {
		body []byte
		err  error
	}
	upstreamDone := make(chan upstreamResult, 1)
	upstreamHeaders := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Permit the provider's early response while continuing to read its upload.
		controller := http.NewResponseController(w)
		if err := controller.EnableFullDuplex(); err != nil {
			upstreamDone <- upstreamResult{err: err}
			http.Error(w, "full duplex unavailable", http.StatusInternalServerError)
			return
		}
		first := make([]byte, len(prefix))
		if _, err := io.ReadFull(r.Body, first); err != nil {
			upstreamDone <- upstreamResult{err: err}
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if err := controller.Flush(); err != nil {
			upstreamDone <- upstreamResult{err: err}
			return
		}
		upstreamHeaders <- struct{}{}
		rest, err := io.ReadAll(r.Body)
		upstreamDone <- upstreamResult{body: append(first, rest...), err: err}
		if err == nil {
			_, _ = io.WriteString(w, responseBody)
		}
	}))
	t.Cleanup(upstream.Close)

	// Send only the prefix. The caller deliberately withholds the tail until
	// it receives AuthProxy's response headers, making response-first observable.
	req := rawEvidenceRequest(t, env, connection, upstream.URL)
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	_ = req.Body.Close()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	req.Body, req.GetBody, req.ContentLength = reader, nil, int64(len(expectedUpload))
	if chunked {
		req.ContentLength = -1
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	uploadDone := make(chan error, 1)
	go func() {
		_, err := writer.Write(prefix)
		if err == nil {
			select {
			case <-release:
				_, err = writer.Write(suffix)
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		_ = writer.CloseWithError(err)
		uploadDone <- err
	}()
	type responseResult struct {
		response *http.Response
		err      error
	}
	headerDone := make(chan responseResult, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		headerDone <- responseResult{resp, err}
	}()
	select {
	case <-upstreamHeaders:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive the prefix and flush its early response")
	}

	// The suffix is still gated, so receiving headers proves AuthProxy did not
	// wait for upload completion before forwarding the upstream response.
	var resp *http.Response
	select {
	case result := <-headerDone:
		require.NoError(t, result.err)
		resp = result.response
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, []string{"sent"}, resp.Header.Values(rawEvidenceHeader))
	case <-time.After(3 * time.Second):
		t.Fatal("raw response headers waited for the unfinished request upload")
	}
	unblock()

	// Finishing the upload must leave both halves of the exchange intact.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, responseBody, string(body))
	require.NoError(t, <-uploadDone)
	completed := <-upstreamDone
	require.NoError(t, completed.err)
	assert.True(t, bytes.Equal(expectedUpload, completed.body), "upstream must receive the exact complete 64 KiB upload")

	// An early response must not snapshot only the upload prefix for capture
	// or freeze the byte counter before an unknown-length upload is complete.
	// The searchable record is written before its blob, so wait for both stores.
	record := waitForLog(t, env, connection, 5*time.Second)
	assert.Equal(t, int64(len(expectedUpload)), record.RequestSizeBytes)
	logCtx, cancelLog := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelLog()
	var captured *app_metrics.FullLog
	require.Eventually(t, func() bool {
		var err error
		captured, err = env.DM.GetAppMetricsService().GetFullLog(logCtx, record.RequestId)
		return err == nil && captured != nil && captured.CaptureLoadState == app_metrics.CaptureLoadStateLoaded
	}, 5*time.Second, 50*time.Millisecond, "full duplex request capture was not persisted")
	assert.Equal(t, int64(len(expectedUpload)), captured.Request.ContentLength)
	if chunked {
		assert.Equal(t, app_metrics.BodySkippedStreaming, record.RequestBodySkipped)
		assert.Equal(t, app_metrics.BodySkippedStreaming, captured.Request.BodySkipped)
		assert.Empty(t, captured.Request.Body)
	} else {
		assert.Empty(t, record.RequestBodySkipped)
		assert.Empty(t, captured.Request.BodySkipped)
		assert.True(t, bytes.Equal(expectedUpload, captured.Request.Body), "capture must contain the complete upload, including its gated tail")
	}
}

// TestRawProxySendEvidence_RequestIsolation verifies concurrent calls on one
// connection cannot borrow each other's completed or in-flight evidence.
func TestRawProxySendEvidence_RequestIsolation(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/drop", helpers.ControlledResponse{DropBeforeHeaders: true})

	// Sequential calls first exercise all states on the same cached connection.
	assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/ok")), 200, "sent")
	assertRawEvidence(t, rawEvidenceRequest(t, env, connection, upstream.URL("/drop")), 500, "mayHaveSent")
	invalid := rawEvidenceRequest(t, env, connection, upstream.URL("/never"))
	invalid.Header.Del("X-AuthProxy-Upstream-URL")
	assertRawEvidence(t, invalid, 400, "notSent")

	// Hold target requests after independent receipt while local rejection finishes.
	release, arrived := make(chan struct{}), make(chan struct{}, 2)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	upstream.Configure("/held-ok", helpers.ControlledResponse{Body: "ok", Arrived: arrived, WaitFor: release})
	upstream.Configure("/held-drop", helpers.ControlledResponse{DropBeforeHeaders: true, Arrived: arrived, WaitFor: release})
	type outcome struct {
		response rawEvidenceResult
		err      error
		drop     bool
	}
	done := make(chan outcome, 2)
	for _, drop := range []bool{false, true} {
		path := "/held-ok"
		if drop {
			path = "/held-drop"
		}
		req := rawEvidenceRequest(t, env, connection, upstream.URL(path))
		go func() {
			resp, err := doRawEvidence(req)
			done <- outcome{resp, err, drop}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent raw requests did not arrive")
		}
	}
	invalid = rawEvidenceRequest(t, env, connection, upstream.URL("/never"))
	invalid.Header.Del("X-AuthProxy-Upstream-URL")
	assertRawEvidence(t, invalid, 400, "notSent")
	unblock()
	for i := 0; i < 2; i++ {
		result := <-done
		require.NoError(t, result.err)
		require.NoError(t, result.response.ReadErr)
		if result.drop {
			assert.Equal(t, 500, result.response.Status)
			assert.Equal(t, "mayHaveSent", result.response.Header.Get(rawEvidenceHeader))
		} else {
			assert.Equal(t, 200, result.response.Status)
			assert.Equal(t, "sent", result.response.Header.Get(rawEvidenceHeader))
		}
	}
	assert.Equal(t, 1, upstream.Count("/held-ok"))
	assert.Equal(t, 1, upstream.Count("/held-drop"))
	assert.Zero(t, upstream.Count("/never"))
}

// TestRawProxySendEvidence_CORS verifies both production services retain their
// exposure policy even when an upstream response supplies a different list.
func TestRawProxySendEvidence_CORS(t *testing.T) {
	const origin = "https://frontend.example.test"
	id := apid.New(apid.PrefixConnector)
	env := helpers.Setup(t, helpers.SetupOptions{
		StartHTTPServer: true, IncludePublic: true,
		Connectors: []sconfig.Connector{helpers.NewNoAuthConnector(id, "raw-evidence-cors", nil)},
		ConfigureRoot: func(root *sconfig.Root) {
			enabled := true
			root.Public.EnableProxyVal = &enabled
			policy := &sconfig.CorsConfig{
				AllowedOrigins: []string{origin}, AllowedMethods: []string{"POST", "OPTIONS"},
				AllowedHeaders: []string{"Authorization", "X-AuthProxy-Upstream-URL"},
				ExposedHeaders: []string{"X-Configured-Exposure"}, AllowCredentials: &enabled,
			}
			root.Api.CorsVal = policy
			root.Public.CorsVal = policy
		},
	})
	t.Cleanup(env.Cleanup)
	connection := env.CreateConnection(t, id, 1)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/cors", helpers.ControlledResponse{
		Headers: http.Header{"Access-Control-Expose-Headers": {"X-Upstream-Only"}, "X-Upstream-Only": {"retained"}},
		Body:    "provider bytes",
	})
	for _, service := range []string{"api", "public"} {
		t.Run(service, func(t *testing.T) {
			auth, base := env.ApiAuthUtil, env.ServerURL
			if service == "public" {
				auth, base = env.PublicAuthUtil, env.PublicURL
			}
			for _, localError := range []bool{false, true} {
				req, err := auth.NewSignedRequestForActorExternalId(http.MethodPost,
					base+"/api/v1/connections/"+connection+"/_proxyRaw", strings.NewReader("cors operation"),
					sconfig.RootNamespace, "raw-cors-actor", aschema.AllPermissions())
				require.NoError(t, err)
				req.Header.Set("Origin", origin)
				if !localError {
					req.Header.Set("X-AuthProxy-Upstream-URL", upstream.URL("/cors"))
				}
				status, evidence := 200, "sent"
				if localError {
					status, evidence = 400, "notSent"
				}

				// Both successful proxying and local errors expose evidence to the
				// allowed browser origin without losing existing configured fields.
				resp := assertRawEvidence(t, req, status, evidence)
				assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"))
				assert.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
				var exposed []string
				for _, value := range resp.Header.Values("Access-Control-Expose-Headers") {
					for _, name := range strings.Split(value, ",") {
						exposed = append(exposed, strings.ToLower(strings.TrimSpace(name)))
					}
				}
				assert.Contains(t, exposed, strings.ToLower(rawEvidenceHeader))
				assert.Contains(t, exposed, "x-configured-exposure")
				if localError {
					assertRawEvidenceError(t, resp)
				} else {
					assert.Contains(t, exposed, "x-upstream-only")
					assert.Equal(t, "retained", resp.Header.Get("X-Upstream-Only"))
					assert.Equal(t, "provider bytes", string(resp.Body))
				}
			}
		})
	}
	assert.Equal(t, 2, upstream.Count("/cors"), "each service dispatches only its valid request")
}
