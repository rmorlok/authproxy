//go:build integration

package oauth2

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 20-24 from issue #179: once OAuth credentials are established,
// proxy-side response handling must preserve upstream responses, refresh and
// replay only for 401, avoid retry loops, and leave connection state intact for
// non-auth upstream failures.

const (
	proxyUpstreamAuthFailureMessage    = "proxy upstream auth failure"
	proxyUpstreamRateLimitedMessage    = "proxy upstream rate limited"
	proxyUpstreamRetryAttemptedMessage = "proxy upstream retry attempted"
)

func resourceRequestsSince(r *proxyRefreshRig, since time.Time) []helpers.RecordedRequest {
	return r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointResource,
		Since:    since,
	})
}

func assertNoRefreshActivity(t *testing.T, rig *proxyRefreshRig) {
	t.Helper()
	assert.Equalf(t, 0, rig.refreshCallCount(),
		"upstream status must not trigger OAuth refresh; got %d refresh-token POSTs", rig.refreshCallCount())
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage),
		"upstream status must not emit refresh-succeeded event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"upstream status must not emit refresh-failed event")
}

func assertConnectionStillReady(t *testing.T, rig *proxyRefreshRig, connID string) {
	t.Helper()
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, conn.State,
		"upstream response classification must not change connection state")
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"upstream response classification must not flip connection unhealthy")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"upstream response classification must not emit health-state transition")
}

func TestProxyResponseHandling_ValidTokenPreservesResponse(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-200")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status: 200,
		Headers: map[string]string{
			"Content-Type":      "application/json",
			"X-Upstream-Trace":  "trace-200",
			"X-Upstream-Status": "ok",
		},
		Body: `{"ok":true,"source":"provider"}`,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, true, resp.BodyJson["ok"])
	assert.Equal(t, "provider", resp.BodyJson["source"])
	assert.Equal(t, "trace-200", resp.Headers["X-Upstream-Trace"])
	assert.Equal(t, "ok", resp.Headers["X-Upstream-Status"])

	resourceReqs := resourceRequestsSince(rig, since)
	require.Lenf(t, resourceReqs, 1, "valid-token request should hit upstream exactly once; got %d", len(resourceReqs))
	authHeader := recordedHeader(resourceReqs[0].Headers, "Authorization")
	require.Truef(t, strings.HasPrefix(authHeader, "Bearer "), "upstream should receive Bearer auth; got %q", authHeader)
	assert.NotEqual(t, "Bearer ", authHeader,
		"upstream must receive a non-empty Bearer token")

	assertNoRefreshActivity(t, rig)
	assertConnectionStillReady(t, rig, connID)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRateLimitedMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage))
}

func TestProxyResponseHandling_401RefreshesAndRetriesOnce(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-401-heal")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status:    401,
		Body:      `{"error":"invalid_token"}`,
		FailCount: 1,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"single upstream 401 should refresh and replay to success; body=%s", w.Body.String())

	resourceReqs := resourceRequestsSince(rig, since)
	assert.Lenf(t, resourceReqs, 2,
		"401 self-heal should call upstream once, refresh, then retry once; got %d resource calls", len(resourceReqs))
	assert.Equalf(t, 1, rig.refreshCallCount(),
		"401 self-heal should perform exactly one refresh-token grant; got %d", rig.refreshCallCount())

	retryEvents := rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage)
	require.Lenf(t, retryEvents, 1, "401 self-heal should emit exactly one retry-attempt event; got %d", len(retryEvents))
	assert.Equal(t, connID, retryEvents[0]["connection_id"])
	assert.Equal(t, float64(401), retryEvents[0]["provider_status_code"])
	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 1,
		"401 self-heal should emit one refresh-succeeded event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage),
		"self-healed 401 should not emit final auth-failure event")
	assertConnectionStillReady(t, rig, connID)
}

func TestProxyResponseHandling_Persistent401RefreshesOnceThenSurfaces(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-401-persistent")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status:    401,
		Body:      `{"error":"not_really_token_related"}`,
		FailCount: 10,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"persistent upstream 401 should surface after one refresh+retry")

	resourceReqs := resourceRequestsSince(rig, since)
	assert.Lenf(t, resourceReqs, 2,
		"persistent 401 must avoid retry loops; expected original + one replay, got %d", len(resourceReqs))
	assert.Equalf(t, 1, rig.refreshCallCount(),
		"persistent 401 should perform exactly one refresh-token grant; got %d", rig.refreshCallCount())

	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage), 1,
		"persistent 401 should emit exactly one retry-attempt event")
	authFailures := rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage)
	require.Lenf(t, authFailures, 1, "persistent final 401 should emit one auth-failure event; got %d", len(authFailures))
	assert.Equal(t, connID, authFailures[0]["connection_id"])
	assert.Equal(t, float64(401), authFailures[0]["provider_status_code"])
	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 1,
		"persistent 401 still refreshes successfully once")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage))
	assertConnectionStillReady(t, rig, connID)
}

func TestProxyResponseHandling_403DoesNotRefresh(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-403")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status: 403,
		Body:   `{"error":"insufficient_scope"}`,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Lenf(t, resourceRequestsSince(rig, since), 1,
		"403 should be returned without upstream retry")
	assertNoRefreshActivity(t, rig)

	authFailures := rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage)
	require.Lenf(t, authFailures, 1, "403 should emit one auth-failure event; got %d", len(authFailures))
	assert.Equal(t, float64(403), authFailures[0]["provider_status_code"])
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage))
	assertConnectionStillReady(t, rig, connID)
}

func TestProxyResponseHandling_429DoesNotRefresh(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-429")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status: 429,
		Headers: map[string]string{
			"Retry-After": "7",
		},
		Body: `{"error":"rate_limited"}`,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "7", resp.Headers["Retry-After"])
	assert.Lenf(t, resourceRequestsSince(rig, since), 1,
		"429 should be returned without upstream retry")
	assertNoRefreshActivity(t, rig)

	rateLimitEvents := rig.logCapture.RecordsWithMessage(t, proxyUpstreamRateLimitedMessage)
	require.Lenf(t, rateLimitEvents, 1, "429 should emit one rate-limit event; got %d", len(rateLimitEvents))
	assert.Equal(t, connID, rateLimitEvents[0]["connection_id"])
	assert.Equal(t, float64(429), rateLimitEvents[0]["provider_status_code"])
	assert.Equal(t, "7", rateLimitEvents[0]["retry_after"])
	assert.Equal(t, float64(7), rateLimitEvents[0]["retry_after_seconds"])
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage))
	assertConnectionStillReady(t, rig, connID)
}

func TestProxyResponseHandling_5xxDoesNotRefreshOrRetry(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-response-5xx")
	connID := rig.completeAuthFlow(t)

	since := time.Now()
	rig.provider.Script("", helpers.EndpointResource, helpers.ScriptAction{
		Status: 503,
		Body:   `{"error":"temporarily_unavailable"}`,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Lenf(t, resourceRequestsSince(rig, since), 1,
		"5xx should be surfaced without upstream retry")
	assertNoRefreshActivity(t, rig)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamAuthFailureMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRateLimitedMessage))
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, proxyUpstreamRetryAttemptedMessage))
	assertConnectionStillReady(t, rig, connID)
}
