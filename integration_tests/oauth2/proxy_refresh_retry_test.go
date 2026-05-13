//go:build integration

package oauth2

import (
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 8 from issue #170: transient retry behavior. When the
// provider's refresh endpoint returns a 5xx response, the proxy must:
//
//   - retry up to `tokenRefreshMaxAttempts` (=3) times with linear
//     backoff before giving up;
//   - on success-after-retry, persist the new token row and emit a
//     refresh-succeeded event (no failure event — transient blips are
//     not alertable);
//   - on retry-budget exhaustion, emit exactly one refresh-failed event
//     with `category=provider_5xx` and `attempts=tokenRefreshMaxAttempts`
//     so dashboards can split exhausted-budget from single non-retryable
//     failures;
//   - on every retried attempt, emit a per-attempt
//     `oauth token refresh transient failure; retrying` warn log line
//     so operators can correlate the latency hit with the retry.
//
// Connection health behavior is the asymmetry with scenario 7: a 5xx
// exhaustion is *transient* (the next proxy call gets another chance),
// so it must NOT flip the connection unhealthy. Only the permanent
// categories (invalid_grant, invalid_client, provider_4xx_other,
// malformed_response, no_refresh_token) flip health — see PR B.

// tokenRefreshRetryMessage mirrors the warn-level per-attempt log line
// emitted by `postRefreshWithRetry`. Operators may key dashboards off
// the attempt count, so the message string is part of the public
// observability contract.
const tokenRefreshRetryMessage = "oauth token refresh transient failure; retrying"

// tokenRefreshMaxAttempts mirrors the production constant in
// `internal/auth_methods/oauth2/proxy.go`. Redeclared here so the test
// fails fast if the production value drifts (the retry-attempt-budget
// assertions would otherwise silently weaken).
const tokenRefreshMaxAttempts = 3

// refreshCallCount returns the number of `grant_type=refresh_token`
// POSTs the test provider has observed for this rig's client.
// `completeAuthFlow` only hits EndpointToken (authorization_code grant),
// not EndpointRefresh — so refresh counts are clean to compare without
// taking a baseline.
func (r *proxyRefreshRig) refreshCallCount() int {
	n := 0
	for _, req := range r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRefresh,
		ClientID: r.clientKey,
	}) {
		if lastForm(req.Form, "grant_type") == "refresh_token" {
			n++
		}
	}
	return n
}

// TestProxyRefresh_TransientRetrySucceeds — provider returns 503 on the
// first two refresh calls, succeeds on the third (FailCount=2 consumes
// two scripted 503s, then the queue falls through to the provider's
// default refresh-token behavior). The proxy's max-attempts budget
// (=tokenRefreshMaxAttempts=3) is exactly enough to ride through the
// transient outage and end up with a fresh access token.
//
// This is the happy-path retry case: the proxy hides the 5xxs from the
// customer's app entirely. Asserts the proxy request succeeds, the new
// token is persisted, health stays healthy, no failure event fires, and
// exactly two retry warn logs are emitted (one per retried attempt).
func TestProxyRefresh_TransientRetrySucceeds(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-retry-success")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	preFailureToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preFailureToken)
	preFailureTokenID := preFailureToken.Id

	// FailCount=2 makes the next two refresh calls return this scripted
	// 503; the third call falls through to a real refresh-token grant.
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status:    503,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 2,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code,
		"proxy must succeed after retried-then-successful refresh; got %d body=%s", w.Code, w.Body.String())

	// New token row persisted with a future expiry — the retried success
	// is observably indistinguishable from a first-try success.
	refreshed := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, refreshed)
	assert.NotEqualf(t, preFailureTokenID, refreshed.Id,
		"retried success must persist a new token row (id should change)")
	require.NotNil(t, refreshed.AccessTokenExpiresAt)

	// Exactly tokenRefreshMaxAttempts refresh calls observed: 2 retried
	// 503s + 1 success. If the proxy gave up early this would be < 3;
	// if it kept retrying past the budget on success, > 3.
	assert.Equalf(t, tokenRefreshMaxAttempts, rig.refreshCallCount(),
		"expected exactly %d refresh-token POSTs (2 retried 503s + 1 success)", tokenRefreshMaxAttempts)

	// One success event, no failure event. The retry-warn lines are
	// fine; the structured failure event is the alertable channel and
	// must stay clean on eventual success.
	succeeded := rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage)
	require.Lenf(t, succeeded, 1, "retried success must emit exactly one refresh-succeeded event; got %d", len(succeeded))
	assert.Equal(t, connID, succeeded[0]["connection_id"])
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"retried success must not emit a refresh-failed event")

	// Two retry-warn logs: one before attempt 2, one before attempt 3.
	// The last attempt never emits a retry log because its outcome is
	// terminal (success here, exhaustion in the test below).
	retryWarns := rig.logCapture.RecordsWithMessage(t, tokenRefreshRetryMessage)
	assert.Lenf(t, retryWarns, 2, "expected 2 retry-warn logs (one per retried attempt); got %d", len(retryWarns))

	// Connection stayed healthy through the transient blips. The success
	// path's MarkHealthState(healthy) is idempotent, so a healthy→healthy
	// transition should not have emitted a "connection health state
	// changed" record either.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"healthy→healthy transitions must not emit a state-change event")
}

// TestProxyRefresh_TransientRetryExhausted — provider returns 503 on
// every refresh call (FailCount=10 is well past the retry budget). The
// proxy retries until tokenRefreshMaxAttempts is exhausted, then
// surfaces the failure.
//
// Distinct shape from scenario 7's permanent failures: the
// provider_5xx category is *transient*, so retry exhaustion must NOT
// flip the connection unhealthy. The next proxy call gets another
// chance. The single failure event records attempts=3 so the alert
// can distinguish exhaustion from a one-shot non-retryable failure.
func TestProxyRefresh_TransientRetryExhausted(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-retry-exhausted")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	preFailureToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preFailureToken)
	preFailureTokenID := preFailureToken.Id
	preFailureEncryptedRefresh := preFailureToken.EncryptedRefreshToken

	// FailCount=10 outlasts the retry budget — the proxy gives up after
	// tokenRefreshMaxAttempts calls and the remaining scripted 503s sit
	// unconsumed.
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status:    503,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.NotEqualf(t, 200, w.Code,
		"proxy must fail when refresh exhausts retry budget; got 200 body=%s", w.Body.String())

	// Exactly one failure event with category=provider_5xx and
	// attempts=tokenRefreshMaxAttempts. The attempts field is the
	// dashboard signal that lets exhausted-budget alerts be tuned
	// differently from single-failure alerts.
	failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
	require.Lenf(t, failed, 1, "exhausted retry must emit exactly one refresh-failed event; got %d (%v)", len(failed), failed)
	event := failed[0]
	assert.Equal(t, "provider_5xx", event["category"])
	assert.Equal(t, connID, event["connection_id"])
	assert.Equal(t, float64(503), event["provider_status_code"])
	assert.Equalf(t, float64(tokenRefreshMaxAttempts), event["attempts"],
		"failure event should record attempts=tokenRefreshMaxAttempts on exhaustion")

	// 5xx is transient — health must NOT flip unhealthy. This is the
	// asymmetry with scenario 7 (which always flips on the first failure
	// for permanent categories). Without this assertion, a regression
	// that reclassified 5xx as permanent would silently pessimize the
	// reconnect UX.
	conn := rig.env.GetConnection(t, connID)
	assert.Equalf(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"transient 5xx exhaustion must not flip the connection unhealthy; got %q", conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"transient 5xx exhaustion must not emit a health-state-changed event")

	// Exactly tokenRefreshMaxAttempts retry warn lines preceded the
	// final failure. The last attempt never logs a retry-warn because
	// there is no further attempt to schedule — so retry-warns count is
	// tokenRefreshMaxAttempts - 1.
	retryWarns := rig.logCapture.RecordsWithMessage(t, tokenRefreshRetryMessage)
	assert.Lenf(t, retryWarns, tokenRefreshMaxAttempts-1,
		"expected %d retry-warn logs (one before each retried attempt); got %d",
		tokenRefreshMaxAttempts-1, len(retryWarns))

	// Token row preservation. A retry exhaustion must not overwrite the
	// stored refresh_token (no partial responses ever land in the DB on
	// 5xx — the response body is unparsed). The pre-failure row remains
	// as forceTokenExpired left it.
	postFailureToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postFailureToken, "retry exhaustion must not delete the token row")
	assert.Equalf(t, preFailureTokenID, postFailureToken.Id,
		"retry exhaustion must not insert a replacement token row")
	assert.Equalf(t, preFailureEncryptedRefresh, postFailureToken.EncryptedRefreshToken,
		"retry exhaustion must not mutate the stored refresh_token")

	// Exactly tokenRefreshMaxAttempts refresh POSTs. If the proxy gave
	// up early this is < 3; if it ignored the budget, > 3.
	assert.Equalf(t, tokenRefreshMaxAttempts, rig.refreshCallCount(),
		"proxy must make exactly tokenRefreshMaxAttempts refresh POSTs before giving up; got %d",
		rig.refreshCallCount())

	// No success event. If a success event fires alongside the failure,
	// the dashboards double-count or worse claim recovery that didn't
	// happen.
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage),
		"failed refresh must not emit a refresh-succeeded event")
}

// TestProxyRefresh_5xxVariants_AllRetried — sanity check that retry
// triggers on the 5xx range, not on a specific status code. Issue #170
// mentions "temporarily_unavailable" via 503 as the canonical example,
// but real providers return 500/502/504 for the same class of issue.
// We exercise 502 to pin a less obvious 5xx — gateway-bad-upstream — as
// retry-eligible, with a single retry that then succeeds.
func TestProxyRefresh_5xxVariants_AllRetried(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-retry-502")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status:    502,
		Body:      `{"error":"bad_gateway"}`,
		FailCount: 1,
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code, "single-retry 502 should succeed on the next attempt; got %d", w.Code)

	// 2 refresh calls observed: the 502 + the recovery success. Anything
	// less means the proxy didn't retry; anything more means the budget
	// was over-spent on a single recoverable blip.
	assert.Equalf(t, 2, rig.refreshCallCount(),
		"expected 2 refresh POSTs (1 retried 502 + 1 success); got %d", rig.refreshCallCount())

	// One retry-warn (between the 502 and the recovery), one success
	// event, no failure event.
	retryWarns := rig.logCapture.RecordsWithMessage(t, tokenRefreshRetryMessage)
	assert.Lenf(t, retryWarns, 1, "expected 1 retry-warn between 502 and success; got %d", len(retryWarns))
	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 1,
		"502→success must emit one refresh-succeeded event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"502→success must not emit a refresh-failed event")
}
