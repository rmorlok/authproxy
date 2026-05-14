//go:build integration

package oauth2

import (
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 9 from issue #171: refresh-token rotation. RFC 6749 §6 allows
// (but does not require) the provider to issue a new `refresh_token` on
// every refresh response. Providers that enforce rotation invalidate the
// previous refresh_token once a new one is issued — re-using the old one
// returns `invalid_grant`. The proxy must:
//
//   - persist any new `refresh_token` from a refresh response (replacing
//     the previous value in the stored token row);
//   - carry the prior `refresh_token` plaintext forward when the provider
//     reuses it across refreshes (RFC 6749 §6 leaves rotation as the
//     provider's choice);
//   - send the *current* `refresh_token` on subsequent refresh POSTs —
//     never an old one. After a rotation, the old refresh_token is dead
//     to the provider and resubmitting it would 400 the next refresh.
//
// We exercise the real go-oauth2-server rotation path rather than scripting
// synthetic tokens because:
//
//   - the test provider redacts `refresh_token` form values in its request
//     recorder, so pinning wire-level RT bytes is impossible — we read the
//     persisted token row and decrypt instead;
//   - scripted access tokens are not valid bearer credentials at the
//     resource server, so the proxied /echo would 401 and trigger the
//     proxy's retry-once-after-refresh path, double-counting refresh POSTs
//     and emitting a refresh-failed event for the second attempt;
//   - the real provider revokes the old refresh_token on rotation (CAS:
//     `WHERE id = ? AND revoked_at IS NULL`) and responds 400 invalid_grant
//     when an old RT is replayed — exactly the signal we want for the
//     forward-chain test, but only available against a real rotation
//     implementation.
//
// The "old refresh token is not restored by stale writes" property
// listed in scenario 9 is a concurrency property — the redis mutex
// around refreshAccessToken prevents a slow concurrent refresh from
// writing back a stale value after a faster one has rotated. That is
// exercised in scenario 10 (PR B for #171).
//
// Because AES-GCM uses a fresh nonce on every encryption, the encrypted
// bytes of the refresh_token column change on every write even when the
// underlying plaintext is identical. Rotation tests therefore decrypt the
// persisted refresh_token and compare *plaintext* values.

// TestProxyRefresh_RotationPersistsNewToken — provider rotates the
// refresh_token (test mode default). The proxy must persist the new
// value into the token row, replacing the existing one. We pin the
// post-refresh decrypted refresh_token against the pre-refresh decrypted
// value — a rotation must produce a *different* plaintext. This is the
// minimal end-to-end correctness check on the rotation path.
func TestProxyRefresh_RotationPersistsNewToken(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-rotation-persist")
	connID := rig.completeAuthFlow(t)
	rig.forceTokenExpired(t, connID, false)

	preRotation := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, preRotation)
	preRotationRefresh := rig.env.DecryptOAuth2RefreshToken(t, preRotation)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code, "proxy must succeed on rotated-token response; got %d body=%s", w.Code, w.Body.String())

	postRotation := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, postRotation, "rotation must leave a token row")
	assert.NotEqualf(t, preRotation.Id, postRotation.Id,
		"rotation must insert a new token row (id should change); got same id %s", postRotation.Id)
	postRotationRefresh := rig.env.DecryptOAuth2RefreshToken(t, postRotation)
	assert.NotEqualf(t, preRotationRefresh, postRotationRefresh,
		"rotation must replace refresh_token plaintext; got identical value (suggests old RT was carried forward)")

	// One success event, no failure event. Rotation must not be alertable.
	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 1,
		"rotation must emit exactly one refresh-succeeded event")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"rotation must not emit a refresh-failed event")
}

// TestProxyRefresh_RotationNextRefreshUsesRotatedToken — the forward-
// chain assertion. After the provider rotates the refresh_token on
// refresh #1, the proxy must send the *rotated* value on refresh #2,
// never the original. This is the assertion that catches a regression
// where the proxy persists the new RT but reads the old one back on
// the next refresh (e.g., cached in memory, or a transaction-isolation
// bug in the token-row lookup).
//
// We rely on the test provider's revocation-on-rotation behaviour:
// replaying a rotated-away refresh_token returns 400 invalid_grant
// (`Refresh token revoked` per `/test/refresh-tokens/rotate-policy`
// docs). So if the proxy regressed and sent the original RT on refresh
// #2, the provider would 400, the proxy would emit a refresh-failed
// event, and the connection would flip unhealthy. The chain succeeding
// twice with the persisted refresh_token plaintext changing each
// time is therefore proof-by-survival that the proxy is reading and
// sending the current RT.
func TestProxyRefresh_RotationNextRefreshUsesRotatedToken(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-rotation-forward-chain")
	connID := rig.completeAuthFlow(t)

	originalRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))

	rig.forceTokenExpired(t, connID, false)
	w1 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w1.Code, "first refresh must succeed; got %d body=%s", w1.Code, w1.Body.String())

	afterFirstRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))
	require.NotEqualf(t, originalRefresh, afterFirstRefresh,
		"refresh #1 must rotate the refresh_token plaintext (fixture sanity)")

	// Refresh #2: if the proxy regressed and replayed the original RT
	// here, the provider would 400 invalid_grant and this proxy call
	// would fail. The chain succeeding is the load-bearing assertion.
	rig.forceTokenExpired(t, connID, false)
	w2 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w2.Code,
		"second refresh must succeed using the rotated refresh_token; got %d body=%s — "+
			"a 4xx here means the proxy replayed the rotated-away RT", w2.Code, w2.Body.String())

	afterSecondRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))
	assert.NotEqualf(t, afterFirstRefresh, afterSecondRefresh,
		"refresh #2 must rotate the refresh_token plaintext again")
	assert.NotEqualf(t, originalRefresh, afterSecondRefresh,
		"rotation chain must never resurrect the original refresh_token")

	// Exactly two refresh-token grants observed on the wire — one per
	// proxy call. A 401-then-refresh retry firing here would inflate this
	// count; the proactive expiry check forecloses that path.
	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 2, "expected exactly 2 refresh-token grants; got %d", len(grants))

	// Two success events, no failure events — proves both refreshes were
	// accepted by the provider.
	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 2,
		"rotation chain must emit exactly two refresh-succeeded events")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"rotation chain must not emit any refresh-failed events")
}

// TestProxyRefresh_NoRotationRetainsRefreshToken — provider has rotation
// disabled. Every refresh response reuses the existing refresh_token, so
// the persisted plaintext must remain the same across the chain. The
// proxy still re-encrypts to a fresh nonce on every write (AES-GCM), so
// the encrypted column bytes change — but the underlying plaintext must
// not.
//
// This pins two related guarantees from RFC 6749 §6 and the proxy's
// internal `createDbTokenFromResponse` logic:
//
//   - when the provider does NOT rotate, the proxy must not synthesize a
//     new refresh_token from nowhere (the next refresh would fail);
//   - the proxy must continue to send a working refresh_token on the
//     next refresh — i.e. the chain survives indefinitely under the
//     provider's no-rotation policy.
func TestProxyRefresh_NoRotationRetainsRefreshToken(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-no-rotation")
	connID := rig.completeAuthFlow(t)

	// Disable rotation on the provider. Test-mode default is on, so
	// restore it on cleanup to avoid leaking state between tests.
	rig.provider.SetRefreshRotation(false)
	t.Cleanup(func() { rig.provider.SetRefreshRotation(true) })

	originalRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))

	rig.forceTokenExpired(t, connID, false)
	w1 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w1.Code, "first refresh must succeed; got %d body=%s", w1.Code, w1.Body.String())

	afterFirstRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))
	assert.Equalf(t, originalRefresh, afterFirstRefresh,
		"no-rotation refresh must preserve refresh_token plaintext across the chain")

	rig.forceTokenExpired(t, connID, false)
	w2 := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w2.Code, "second refresh must succeed; got %d body=%s", w2.Code, w2.Body.String())

	afterSecondRefresh := rig.env.DecryptOAuth2RefreshToken(t, rig.env.GetOAuth2Token(t, connID))
	assert.Equalf(t, originalRefresh, afterSecondRefresh,
		"no-rotation chain must leave refresh_token plaintext unchanged across all refreshes")

	grants := refreshGrantRequests(rig)
	assert.Lenf(t, grants, 2, "expected exactly 2 refresh-token grants; got %d", len(grants))

	require.Lenf(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage), 2,
		"no-rotation chain must emit exactly two refresh-succeeded events")
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"no-rotation chain must not emit any refresh-failed events")
}

// refreshGrantRequests returns just the `grant_type=refresh_token` POSTs
// observed by the test provider for this rig's client, in the order
// they arrived.
func refreshGrantRequests(r *proxyRefreshRig) []helpers.RecordedRequest {
	out := []helpers.RecordedRequest{}
	for _, req := range r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRefresh,
		ClientID: r.clientKey,
	}) {
		if lastForm(req.Form, "grant_type") == "refresh_token" {
			out = append(out, req)
		}
	}
	return out
}
