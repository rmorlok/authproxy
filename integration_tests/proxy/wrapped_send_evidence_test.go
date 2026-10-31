//go:build integration

package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wrappedEvidenceResponse reads the public JSON contract independently of the
// server's DTO, so missing or incorrectly nested evidence fails these tests.
type wrappedEvidenceResponse struct {
	StatusCode   int               `json:"statusCode"`
	Headers      map[string]string `json:"headers"`
	BodyRaw      []byte            `json:"bodyRaw"`
	BodyJSON     json.RawMessage   `json:"bodyJson"`
	Error        string            `json:"error"`
	SendEvidence string            `json:"sendEvidence"`
}

// newWrappedEvidenceEnv starts production API routes on a real TCP listener.
// Each suite receives isolated database state and a unique connection, while
// the production Redis-backed rate limiter remains active when requested.
func newWrappedEvidenceEnv(t *testing.T, rateLimiting bool) (*helpers.IntegrationTestEnv, string) {
	t.Helper()
	id := apid.New(apid.PrefixConnector)
	env := helpers.Setup(t, helpers.SetupOptions{
		StartHTTPServer: true,
		Connectors: []sconfig.Connector{helpers.NewNoAuthConnector(id, "send-evidence", &connectors.RateLimiting{
			Disabled: !rateLimiting,
		})},
	})
	t.Cleanup(env.Cleanup)
	return env, env.CreateConnection(t, id, 1)
}

// wrappedEvidenceRequest signs a replayable POST with a distinctive payload.
// Tests prepare requests before starting goroutines so JWT creation and setup
// do not introduce unrelated concurrent database mutations.
func wrappedEvidenceRequest(t *testing.T, env *helpers.IntegrationTestEnv, connection, target string) *http.Request {
	t.Helper()
	body, err := json.Marshal(iface.ProxyRequest{URL: target, Method: http.MethodPost, BodyRaw: []byte("recorded operation")})
	require.NoError(t, err)
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(http.MethodPost,
		env.ServerURL+"/api/v1/connections/"+connection+"/_proxy", bytes.NewReader(body),
		sconfig.RootNamespace, "evidence-actor", aschema.AllPermissions())
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// replaceEvidenceBody changes only the API payload, retaining authentication
// and transport framing for validation-error scenarios.
func replaceEvidenceBody(req *http.Request, body string) {
	req.Body = io.NopCloser(strings.NewReader(body))
	req.ContentLength = int64(len(body))
	// The body is deliberately single-use; the API test client never retries.
	req.GetBody = nil
}

// doWrappedEvidence performs a real HTTP request without testing.T calls, so
// concurrent scenarios can report failures back to their owning test goroutine.
func doWrappedEvidence(req *http.Request) (wrappedEvidenceResponse, int, error) {
	var result wrappedEvidenceResponse
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return result, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		err = json.Unmarshal(body, &result)
	}
	if err != nil {
		return result, resp.StatusCode, fmt.Errorf("read proxy response %q: %w", body, err)
	}
	return result, resp.StatusCode, nil
}

// assertWrappedEvidence checks both envelope status and top-level evidence.
func assertWrappedEvidence(t *testing.T, req *http.Request, status int, evidence string) wrappedEvidenceResponse {
	t.Helper()
	resp, gotStatus, err := doWrappedEvidence(req)
	require.NoError(t, err)
	require.Equal(t, status, gotStatus, "response: %+v", resp)
	assert.Equal(t, evidence, resp.SendEvidence)
	return resp
}

// TestWrappedProxySendEvidence_UpstreamResponses verifies errors from the
// target remain wrapped responses with confirmed sends, just like success.
func TestWrappedProxySendEvidence_UpstreamResponses(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, status := range []int{200, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			path := fmt.Sprintf("/status/%d", status)
			upstream.Configure(path, helpers.ControlledResponse{
				Status: status,
				Headers: http.Header{
					"Content-Type":              {"application/json"},
					"X-Authproxy-Send-Evidence": {"notSent"},
				},
				Body: `{"upstream":true,"sendEvidence":"notSent"}`,
			})

			// The API's outer 200 and provider status/body retain their old shape.
			resp := assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL(path)), 200, "sent")
			assert.Equal(t, status, resp.StatusCode)
			// Provider-controlled body/header claims stay nested and cannot
			// replace AuthProxy's trusted top-level transport evidence.
			assert.JSONEq(t, `{"upstream":true,"sendEvidence":"notSent"}`, string(resp.BodyJSON))
			assert.Equal(t, "notSent", resp.Headers["X-Authproxy-Send-Evidence"])
			recorded := upstream.Requests(path)
			require.Len(t, recorded, 1)
			assert.Equal(t, http.MethodPost, recorded[0].Method)
			assert.Equal(t, "recorded operation", recorded[0].Body)
		})
	}
}

// TestWrappedProxySendEvidence_LocalRejections verifies metadata is installed
// before authentication and survives ordinary auth, permission, and route errors.
func TestWrappedProxySendEvidence_LocalRejections(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, scenario := range []struct {
		name   string
		status int
		mutate func(*http.Request)
	}{
		{"missing-auth", 401, func(r *http.Request) { r.Header.Del("Authorization") }},
		{"invalid-auth", 401, func(r *http.Request) { r.Header.Set("Authorization", "Bearer invalid") }},
		{"malformed-json", 400, func(r *http.Request) { replaceEvidenceBody(r, "{") }},
		{"missing-url", 400, func(r *http.Request) { replaceEvidenceBody(r, `{"method":"GET"}`) }},
		{"invalid-method", 400, func(r *http.Request) { replaceEvidenceBody(r, `{"url":"http://unused.test","method":"INVALID"}`) }},
		{"invalid-connection", 400, func(r *http.Request) { r.URL.Path = "/api/v1/connections/invalid/_proxy" }},
		{"missing-connection", 404, func(r *http.Request) {
			r.URL.Path = "/api/v1/connections/" + apid.New(apid.PrefixConnection).String() + "/_proxy"
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			req := wrappedEvidenceRequest(t, env, connection, upstream.URL("/must-not-send"))
			scenario.mutate(req)

			// All these failures occur before target dispatch and retain errors.
			resp := assertWrappedEvidence(t, req, scenario.status, "notSent")
			assert.NotEmpty(t, resp.Error)
			assert.Zero(t, upstream.Count(""))
		})
	}
	t.Run("missing-permission", func(t *testing.T) {
		req := wrappedEvidenceRequest(t, env, connection, upstream.URL("/must-not-send"))
		token, err := env.ApiAuthUtil.GenerateBearerToken(req.Context(), "read-only-evidence-actor", sconfig.RootNamespace,
			[]aschema.Permission{{Namespace: "root.**", Resources: []string{"connections"}, Verbs: []string{"get"}}})
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		// A valid identity without the proxy verb must also report unsent.
		resp := assertWrappedEvidence(t, req, 403, "notSent")
		assert.NotEmpty(t, resp.Error)
		assert.Zero(t, upstream.Count(""))
	})
}

// TestWrappedProxySendEvidence_RateLimit distinguishes an upstream 429 from
// the following identical status synthesized by AuthProxy's real rate limiter.
func TestWrappedProxySendEvidence_RateLimit(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, true)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/rate-limit", helpers.ControlledResponse{
		Status: 429, Headers: http.Header{"Retry-After": {"60"}}, Body: "provider limited",
	})

	// Prime a long backoff through a genuine provider response, without sleeps.
	first := assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/rate-limit")), 200, "sent")
	assert.Equal(t, 429, first.StatusCode)
	require.Equal(t, 1, upstream.Count(""))

	// Change the upstream to success: the next 429 can only be produced locally.
	upstream.Configure("/rate-limit", helpers.ControlledResponse{Status: 200, Body: "allowed"})
	blocked := assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/rate-limit")), 200, "notSent")
	assert.Equal(t, 429, blocked.StatusCode)
	assert.Equal(t, "true", blocked.Headers["X-Authproxy-Ratelimited"])
	assert.Equal(t, 1, upstream.Count(""))
}

// TestWrappedProxySendEvidence_ResponseFailures observes transport failures
// and later decoding failures without deriving evidence from the final status.
func TestWrappedProxySendEvidence_ResponseFailures(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	for _, scenario := range []struct {
		name     string
		action   helpers.ControlledResponse
		status   int
		evidence string
	}{
		{"drop", helpers.ControlledResponse{DropBeforeHeaders: true}, 500, "mayHaveSent"},
		{"malformed-json", helpers.ControlledResponse{Headers: http.Header{"Content-Type": {"application/json"}}, Body: "{invalid"}, 500, "sent"},
		{"truncated", helpers.ControlledResponse{TruncateAfterHeaders: true, Body: "partial"}, 200, "sent"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := "/failure/" + scenario.name
			upstream.Configure(path, scenario.action)

			// The upstream independently records the POST before dropping its
			// response; mayHaveSent therefore includes operations that did arrive.
			resp := assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL(path)), scenario.status, scenario.evidence)
			recorded := upstream.Requests(path)
			require.Len(t, recorded, 1)
			assert.Equal(t, "recorded operation", recorded[0].Body)
			if scenario.name == "truncated" {
				// Preserve the wrapped Bytes() conversion's existing behavior:
				// body truncation does not surface as an outer API error.
				assert.Equal(t, 200, resp.StatusCode)
			} else {
				assert.NotEmpty(t, resp.Error)
			}
		})
	}

	t.Run("redirect-then-drop", func(t *testing.T) {
		upstream.Configure("/redirect", helpers.ControlledResponse{
			// The redirect changes POST to GET. Closing this first socket avoids
			// net/http transparently retrying a GET on a reused connection.
			Status: 302, Headers: http.Header{
				"Location": {upstream.URL("/redirect-drop")}, "Connection": {"close"},
			},
		})
		upstream.Configure("/redirect-drop", helpers.ControlledResponse{DropBeforeHeaders: true})

		// Even though the redirected request fails, the initial 302 confirms send.
		assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/redirect")), 500, "sent")
		assert.Equal(t, 1, upstream.Count("/redirect"))
		assert.Equal(t, 1, upstream.Count("/redirect-drop"))
	})
}

// TestWrappedProxySendEvidence_RedirectThenLocalBlock establishes a backoff
// while the redirect is in flight, proving a later locally rejected hop cannot
// erase the first response's evidence.
func TestWrappedProxySendEvidence_RedirectThenLocalBlock(t *testing.T) {
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
	req := wrappedEvidenceRequest(t, env, connection, upstream.URL("/redirect"))
	type outcome struct {
		response wrappedEvidenceResponse
		status   int
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		resp, status, err := doWrappedEvidence(req)
		done <- outcome{resp, status, err}
	}()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("initial redirect request did not arrive")
	}

	// A second logical request establishes the real Redis-backed suppression
	// before the first request receives its redirect and follows the next hop.
	assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/prime-backoff")), 200, "sent")
	unblock()
	result := <-done
	require.NoError(t, result.err)
	assert.Equal(t, 200, result.status)
	assert.Equal(t, 429, result.response.StatusCode)
	assert.Equal(t, "sent", result.response.SendEvidence)
	assert.Equal(t, 1, upstream.Count("/redirect"))
	assert.Equal(t, 1, upstream.Count("/prime-backoff"))
	assert.Zero(t, upstream.Count("/blocked-hop"))
}

// TestWrappedProxySendEvidence_RequestIsolation keeps several responses in
// flight while a separate request is rejected, then verifies per-request state.
func TestWrappedProxySendEvidence_RequestIsolation(t *testing.T) {
	env, connection := newWrappedEvidenceEnv(t, false)
	upstream := helpers.NewControlledUpstream(t)
	upstream.Configure("/drop", helpers.ControlledResponse{DropBeforeHeaders: true})

	// Sequential calls on the same connection must neither retain previous sent
	// evidence nor let a failed call poison the next successful observation.
	assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/ok")), 200, "sent")
	assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/drop")), 500, "mayHaveSent")
	invalid := wrappedEvidenceRequest(t, env, connection, upstream.URL("/never"))
	replaceEvidenceBody(invalid, `{"method":"GET"}`)
	assertWrappedEvidence(t, invalid, 400, "notSent")
	assertWrappedEvidence(t, wrappedEvidenceRequest(t, env, connection, upstream.URL("/ok")), 200, "sent")

	// Hold four target attempts after receipt so they necessarily overlap.
	release, arrived := make(chan struct{}), make(chan struct{}, 4)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	upstream.Configure("/concurrent-ok", helpers.ControlledResponse{Body: "ok", Arrived: arrived, WaitFor: release})
	upstream.Configure("/concurrent-drop", helpers.ControlledResponse{DropBeforeHeaders: true, Arrived: arrived, WaitFor: release})
	type outcome struct {
		response wrappedEvidenceResponse
		status   int
		err      error
		drop     bool
	}
	results := make(chan outcome, 4)
	for i := 0; i < 4; i++ {
		drop := i%2 == 0
		path := "/concurrent-ok"
		if drop {
			path = "/concurrent-drop"
		}
		req := wrappedEvidenceRequest(t, env, connection, upstream.URL(path))
		go func() {
			resp, status, err := doWrappedEvidence(req)
			results <- outcome{resp, status, err, drop}
		}()
	}
	for i := 0; i < 4; i++ {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent target requests did not arrive")
		}
	}

	// Local rejection completes while the other calls are still held upstream.
	invalid = wrappedEvidenceRequest(t, env, connection, upstream.URL("/never"))
	replaceEvidenceBody(invalid, `{"method":"GET"}`)
	assertWrappedEvidence(t, invalid, 400, "notSent")
	unblock()
	for i := 0; i < 4; i++ {
		result := <-results
		require.NoError(t, result.err)
		if result.drop {
			assert.Equal(t, 500, result.status)
			assert.Equal(t, "mayHaveSent", result.response.SendEvidence)
		} else {
			assert.Equal(t, 200, result.status)
			assert.Equal(t, "sent", result.response.SendEvidence)
		}
	}
	assert.Equal(t, 2, upstream.Count("/concurrent-ok"))
	assert.Equal(t, 2, upstream.Count("/concurrent-drop"))
	assert.Zero(t, upstream.Count("/never"))
}
