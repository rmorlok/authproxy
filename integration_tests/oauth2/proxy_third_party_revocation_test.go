//go:build integration

package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/database"
)

// proxyResponseBody mirrors core/iface.ProxyResponse — the JSON shape
// the proxy API wraps an upstream response in. The API itself returns
// 200 when the proxy plumbing succeeds; the *upstream* status lives in
// the inner status_code field. Most refresh tests in this package only
// need to assert success vs. failure at the API layer (the wrapper
// returns non-200 only on proxy infrastructure errors), but revocation
// tests must inspect the upstream status to tell apart 200 (self-heal
// succeeded) from 401 (refresh-then-replay failed, original upstream
// 401 propagated).
type proxyResponseBody struct {
	StatusCode int                    `json:"status_code"`
	Headers    map[string]string      `json:"headers"`
	BodyRaw    []byte                 `json:"body_raw"`
	BodyJson   map[string]interface{} `json:"body_json"`
}

func parseRevocationProxyResponse(t *testing.T, w *httptest.ResponseRecorder) proxyResponseBody {
	t.Helper()
	require.Equalf(t, http.StatusOK, w.Code,
		"proxy endpoint must return 200 (proxy plumbing succeeded); got %d body=%s", w.Code, w.Body.String())
	var resp proxyResponseBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "decode proxy response body")
	return resp
}

// Scenario 11 from issue #172: third-party revocation detection. After
// the user revokes access at the provider, the proxy must detect the
// dead credential on its next request, surface it as a reconnect-
// required failure, and stop using the dead refresh token. The proxy
// has two natural detection paths:
//
//   - 401 from the upstream resource server when the access token was
//     revoked (the access token still looks valid clock-wise to the
//     proxy, but the provider rejects it). This routes through
//     ProxyRequest's retry-once-after-refresh path: refresh, replay
//     /echo with the new access token, return the replay result.
//   - 400 from the refresh endpoint when the refresh token was revoked
//     (cascaded from a user-level revocation, or revoked directly).
//     This routes through classifyAndRecordRefreshFailure and flips
//     the connection unhealthy.
//
// The two paths combine: a full user revocation (RevokeUser) kills
// both access + refresh tokens, so the 401-then-refresh path fires
// AND the refresh leg fails. An access-only revocation (RevokeToken
// on the access plaintext) leaves the refresh leg intact, so the
// proxy self-heals — the refreshed access token is a fresh, valid
// credential and the replay succeeds.
//
// Why real provider revocation, not scripted invalid_grant
//
// Scenario 7's InvalidGrant case already pins the proxy's response to
// a 400 invalid_grant on the refresh endpoint when that response is
// scripted. Scenario 11 is the *causal chain* version: the provider's
// natural revocation state machine — not a scripted response — turns a
// previously-working credential into a refresh failure, and the proxy
// must detect and surface that. Driving this through the test
// provider's /test/revoke control plane validates the full causal
// chain (access-token-revoked → 401 at /echo → proxy refreshes →
// refresh-token-revoked → 400 → unhealthy) without scripted
// shortcuts.
//
// Fixture caveat on the failure category
//
// RFC 6749 §5.2 specifies that providers return `{"error":"invalid_grant"}`
// for a revoked refresh token, and the proxy's classifier therefore
// has an `invalid_grant` category for that case. The go-oauth2-server
// test provider, however, returns `{"error":"Refresh token revoked"}`
// (a non-standard string baked into its `ErrRefreshTokenRevoked`
// sentinel). The proxy's classifier sees a string it doesn't recognize
// and categorizes the failure as `provider_4xx_other`, with the
// health-state transition reason `refresh_provider_4xx_other`. Real
// providers issue RFC-compliant `invalid_grant` — scenario 7's
// scripted-response test covers that mapping. Scenario 11's assertions
// reflect what the fixture actually emits.

// TestProxyRevocation_UserRevocationFlipsUnhealthy — RevokeUser kills
// both the access and refresh tokens at the provider. The next proxy
// request triggers the 401-then-refresh-then-fail chain:
//
//   - /echo with the stored access_token → 401 invalid_token
//     (the access token is revoked).
//   - ProxyRequest enters retry-once-after-refresh, posts
//     grant_type=refresh_token to /token. The refresh token is
//     revoked too, so the provider returns 400 with its non-RFC error
//     string "Refresh token revoked".
//   - classifyAndRecordRefreshFailure emits a refresh-failed event
//     and flips the connection unhealthy via MarkHealthState.
//   - The retry replay never fires (refresh failed) and the original
//     401 is returned to the caller — exactly the reconnect-required
//     signal scenario 11 requires.
//
// A second proxy request after the first must continue to fail and
// must NOT emit another health-state-changed transition event
// (MarkHealthState is idempotent for unhealthy→unhealthy).
func TestProxyRevocation_UserRevocationFlipsUnhealthy(t *testing.T) {
	rig := newProxyRefreshRig(t, "revocation-user")
	connID := rig.completeAuthFlow(t)

	// Revoke ALL tokens for this user at the provider. The proxy's
	// stored access_token has not yet been used after this point — it
	// looks fresh to the proxy clock-wise, so the only way to detect
	// the revocation is via an upstream 401.
	rig.provider.RevokeUser(rig.userID)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equalf(t, http.StatusUnauthorized, resp.StatusCode,
		"revoked access token + revoked refresh token must surface as upstream 401; got %d body=%s", resp.StatusCode, w.Body.String())

	// Health flipped to unhealthy with the reason encoding the refresh
	// category — exactly the field dashboards correlate the refresh
	// failure event against.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
		"user revocation must flip the connection unhealthy")

	failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
	require.Lenf(t, failed, 1,
		"expected exactly one refresh-failed event from the 401-then-refresh path; got %d", len(failed))
	// Fixture caveat: the go-oauth2-server test provider returns
	// {"error":"Refresh token revoked"} on a revoked refresh token rather
	// than RFC 6749 §5.2's {"error":"invalid_grant"}. The proxy's
	// classifier therefore categorizes this as `provider_4xx_other`, not
	// `invalid_grant`. Real providers return `invalid_grant` — scenario 7
	// (`proxy_refresh_test.go`'s InvalidGrant case) covers the
	// RFC-compliant path with a scripted response. This test covers the
	// causal chain (revoke → upstream 401 → refresh → 400 → unhealthy)
	// end-to-end against the provider's actual revocation state machine,
	// so we assert what the provider actually returns.
	assert.Equal(t, "provider_4xx_other", failed[0]["category"],
		"category reflects the test provider's non-RFC error string ('Refresh token revoked')")
	assert.Equal(t, connID, failed[0]["connection_id"])
	assert.Equal(t, float64(400), failed[0]["provider_status_code"])
	assert.Equal(t, "Refresh token revoked", failed[0]["provider_error"])

	transitions := rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage)
	require.Lenf(t, transitions, 1,
		"expected exactly one health-state-changed event on first detection; got %d", len(transitions))
	assert.Equal(t, "healthy", transitions[0]["previous_health_state"])
	assert.Equal(t, "unhealthy", transitions[0]["health_state"])
	assert.Equal(t, "refresh_provider_4xx_other", transitions[0]["reason"])

	// Exactly one refresh-token POST in this leg. Filtered to
	// EndpointRefresh + grant_type=refresh_token so the
	// authorization-code POST from completeAuthFlow is excluded.
	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 1,
		"expected exactly 1 refresh-token POST on the 401-then-refresh path; got %d", len(grants))

	// Token row preservation. The refresh failed, so no replacement
	// row should have been inserted — the existing row remains with
	// its access_token (now revoked at the provider) and its
	// refresh_token (also revoked).
	postToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postToken, "failed refresh must not delete the token row")

	// A second proxy request must continue to fail. The 401-retry
	// path will fire again and produce a second refresh-failed
	// event, but MarkHealthState(unhealthy → unhealthy) is
	// idempotent so the transition event count stays at 1 — that
	// is the load-bearing signal for dashboards that don't want
	// every replay attempt to look like a new event.
	w2 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp2 := parseRevocationProxyResponse(t, w2)
	assert.Equalf(t, http.StatusUnauthorized, resp2.StatusCode,
		"future proxy requests after revocation must continue to fail with upstream 401; got %d", resp2.StatusCode)
	conn = rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
		"unhealthy connection must stay unhealthy across retries")
	transitions2 := rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage)
	assert.Lenf(t, transitions2, 1,
		"unhealthy→unhealthy must be idempotent; expected still exactly one transition event, got %d", len(transitions2))
}

// TestProxyRevocation_AccessOnlyRevocationSelfHealsViaRefresh — the
// provider revokes only the access token (RevokeToken on the access-
// token plaintext does not cascade to the refresh token, per
// AdminRevokeByToken in the test provider). The proxy's 401-then-
// refresh path must:
//
//   - detect the upstream 401 at /echo;
//   - refresh successfully (the refresh_token is still valid);
//   - replay /echo with the new access token, returning 200 to the
//     caller — observable self-heal.
//
// This pins the recoverable subset of scenario 11: when only the
// access leg is dead, the proxy must not flip the connection
// unhealthy — that would train users into reconnect prompts the
// proxy could have resolved transparently.
//
// Note: the test provider's default rotation policy is on, so the
// refresh response also rotates the refresh_token. The new
// refresh_token must replace the stored plaintext (PR A's rotation
// guarantee, re-exercised here under the 401-retry path).
func TestProxyRevocation_AccessOnlyRevocationSelfHealsViaRefresh(t *testing.T) {
	rig := newProxyRefreshRig(t, "revocation-access-only")
	connID := rig.completeAuthFlow(t)

	preToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preToken)
	preTokenID := preToken.Id
	preRefreshPlaintext := rig.env.DecryptOAuth2RefreshToken(t, preToken)
	accessTokenPlaintext := rig.env.DecryptOAuth2AccessToken(t, preToken)

	// Revoke ONLY the access token. The refresh token stays valid,
	// so the refresh leg of the proxy's 401-retry path will succeed
	// and mint fresh credentials.
	rig.provider.RevokeToken(accessTokenPlaintext)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"access-only revocation must self-heal via refresh + replay; got upstream %d body=%s", resp.StatusCode, w.Body.String())

	// Connection stays healthy — no transition event, no failure event.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"self-heal must leave the connection healthy")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"self-heal must not emit a refresh-failed event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"self-heal must not emit a health-state-changed event (healthy→healthy is idempotent)")

	// Exactly one refresh-succeeded event for the refresh that
	// resolved the 401.
	succeeded := rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage)
	require.Lenf(t, succeeded, 1,
		"self-heal must emit exactly one refresh-succeeded event; got %d", len(succeeded))
	assert.Equal(t, connID, succeeded[0]["connection_id"])

	// Exactly one refresh-token POST on the wire — the 401-retry
	// path fires once and the replay succeeds, so no further refresh
	// is needed.
	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 1,
		"expected exactly 1 refresh-token POST; got %d", len(grants))

	// Token row rotated forward: new row id, new refresh_token
	// plaintext (test provider default rotation policy is on).
	postToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postToken)
	assert.NotEqualf(t, preTokenID, postToken.Id,
		"successful refresh must persist a new token row; pre id=%s post id=%s", preTokenID, postToken.Id)
	postRefreshPlaintext := rig.env.DecryptOAuth2RefreshToken(t, postToken)
	assert.NotEqualf(t, preRefreshPlaintext, postRefreshPlaintext,
		"successful refresh must rotate the refresh_token plaintext (provider default policy)")

	// A second proxy request with the now-fresh access token must
	// succeed without triggering another refresh. This pins that the
	// self-heal stuck — the new access token works at /echo on its
	// own.
	w2 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp2 := parseRevocationProxyResponse(t, w2)
	assert.Equalf(t, http.StatusOK, resp2.StatusCode,
		"second proxy request after self-heal must succeed without further refresh; got upstream %d", resp2.StatusCode)
	grants2 := refreshGrantRequests(rig)
	assert.Lenf(t, grants2, 1,
		"second proxy request must not trigger another refresh; expected still 1 grant, got %d", len(grants2))
}

// Sanity guard against a future test-provider change: if RevokeToken
// were ever changed to cascade access→refresh, the "access-only" test
// above would silently regress into the "user revocation" case. Pin
// the non-cascade behavior at the boundary so the regression
// surfaces as a clear failure here rather than as a confusing
// invalid_grant in the self-heal test.
func TestProxyRevocation_AccessTokenRevokeDoesNotCascadeToRefresh(t *testing.T) {
	rig := newProxyRefreshRig(t, "revocation-no-cascade")
	connID := rig.completeAuthFlow(t)

	preToken := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preToken)
	accessTokenPlaintext := rig.env.DecryptOAuth2AccessToken(t, preToken)
	refreshTokenPlaintext := rig.env.DecryptOAuth2RefreshToken(t, preToken)
	require.NotEqual(t, accessTokenPlaintext, refreshTokenPlaintext,
		"fixture sanity: provider must mint distinct access and refresh tokens")

	rig.provider.RevokeToken(accessTokenPlaintext)

	// Forge-expire the access token locally so the proxy uses the
	// proactive refresh path (not the 401-retry path) and refreshes
	// directly with the still-valid refresh token. If the access-
	// token revoke had cascaded to the refresh token, this would
	// fail with invalid_grant and the connection would flip
	// unhealthy.
	rig.forceTokenExpired(t, connID, false)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	resp := parseRevocationProxyResponse(t, w)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"after access-only revocation, refresh-token grant must still succeed (no cascade); got upstream %d body=%s",
		resp.StatusCode, w.Body.String())
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"non-cascading revoke must not produce a refresh failure")
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState,
		"non-cascading revoke must not flip the connection unhealthy")

	// One refresh-token POST. The provider's response is the real
	// rotation-on grant, and the resulting access_token is fresh
	// (not the revoked one), so /echo succeeds without any 401-
	// retry detour.
	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 1, "expected exactly 1 refresh-token POST; got %d", len(grants))
}
