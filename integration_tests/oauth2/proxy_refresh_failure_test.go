//go:build integration

package oauth2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 7 from issue #170: deterministic refresh failures. The proxy
// detects an expired access token, POSTs a refresh-token grant to the
// provider, and the provider returns a permanent failure (a 4xx with one
// of the RFC 6749 §5.2 error codes, a 4xx with no recognized code, a
// malformed 200 body, or a 200 with no access_token). Every case must:
//
//   - emit exactly one structured "oauth token refresh failed" event with
//     the right `category` (the dashboards key off this string), with the
//     `provider_status_code` and `provider_error` fields populated when
//     applicable;
//   - flip the connection's `health_state` to `unhealthy` and emit a
//     single `connection health state changed` event with `reason` of
//     `refresh_<category>` (so the dashboards correlate the two);
//   - NOT corrupt the persisted token row — a malformed refresh response
//     must not overwrite the existing refresh_token with garbage;
//   - cause the original proxy request to fail (no valid access token →
//     no proxy);
//   - hit the refresh endpoint exactly once. None of the scenario 7 cases
//     are retryable, so the retry loop must short-circuit on the first
//     4xx / unparseable-200 / no-access-token response.
//
// The 5xx / transient cases (scenario 8) are PR C and live in
// proxy_refresh_retry_test.go.

// refreshFailureCase describes one scripted refresh-endpoint response and
// the observable shape the proxy must produce. The fixture differs only
// in the scripted action and the expected category/status; every other
// lever (connector, auth flow, expiry forge, proxy call) is identical.
type refreshFailureCase struct {
	name                string
	scriptAction        helpers.ScriptAction
	expectedCategory    string
	expectedStatusCode  int    // 0 = assert field omitted on the event
	expectedProviderErr string // "" = assert field omitted on the event
}

// scenario 7 cases, mapped one-to-one onto the categories enumerated in
// `internal/auth_methods/oauth2/token_refresh_failure.go`. The mapping is
// load-bearing for dashboards — changing any of these category strings
// is a public observability break.
var refreshFailureCases = []refreshFailureCase{
	{
		name:                "InvalidGrant",
		scriptAction:        helpers.ScriptAction{Status: 400, Body: `{"error":"invalid_grant"}`},
		expectedCategory:    "invalid_grant",
		expectedStatusCode:  400,
		expectedProviderErr: "invalid_grant",
	},
	{
		// Per RFC 6749 §5.2 a 401 with WWW-Authenticate is the canonical
		// invalid_client shape; the proxy classifies on the body, so the
		// header is irrelevant to this test.
		name:                "InvalidClient",
		scriptAction:        helpers.ScriptAction{Status: 401, Body: `{"error":"invalid_client"}`},
		expectedCategory:    "invalid_client",
		expectedStatusCode:  401,
		expectedProviderErr: "invalid_client",
	},
	{
		// Non-spec 4xx — typical of a WAF / rate-limiter page sitting in
		// front of the provider's token endpoint. provider_error is
		// omitted because there is nothing safe to log.
		name: "Provider4xxOther",
		scriptAction: helpers.ScriptAction{
			Status:  403,
			Headers: map[string]string{"Content-Type": "text/html"},
			Body:    "<html><body>Forbidden by WAF</body></html>",
		},
		expectedCategory:   "provider_4xx_other",
		expectedStatusCode: 403,
	},
	{
		// 200 with an unparseable body — distinct from any 4xx. The
		// request "succeeded" at the HTTP layer but the proxy could not
		// extract an access_token, so the refresh is observably broken
		// without being attributable to a known §5.2 cause.
		name:               "MalformedResponse",
		scriptAction:       helpers.ScriptAction{Status: 200, BodyTemplate: helpers.BodyMalformedJSON},
		expectedCategory:   "malformed_response",
		expectedStatusCode: 200,
	},
	{
		// 200 with a well-formed JSON envelope that omits `access_token`.
		// `createDbTokenFromResponse` rejects this as a malformed
		// response — the proxy has no useful credential to persist. Same
		// observable category as the unparseable-body case (issue #189
		// folds into this case).
		name:               "NoAccessTokenInResponse",
		scriptAction:       helpers.ScriptAction{Status: 200, Body: `{"token_type":"Bearer","expires_in":3600}`},
		expectedCategory:   "malformed_response",
		expectedStatusCode: 200,
	},
}

// TestProxyRefreshFailure_Scenarios drives every scripted refresh-endpoint
// failure through the same fixture and asserts the full observable shape:
// health flip, structured events, token-row preservation, no excess
// refresh calls.
//
// Each subtest stands up its own rig because the test provider's request
// log + script queues are per-fixture, and because cross-subtest leakage
// of LogCapture records would make the "exactly one failure event"
// assertion meaningless. Subtest startup is dominated by container reuse,
// so the cost is negligible compared to the clarity benefit.
func TestProxyRefreshFailure_Scenarios(t *testing.T) {
	for _, tc := range refreshFailureCases {
		t.Run(tc.name, func(t *testing.T) {
			// The test provider lowercases registered client keys, so the
			// rig name is normalized; the subtest name stays CamelCase for
			// readability in test output.
			rig := newProxyRefreshRig(t, fmt.Sprintf("refresh-fail-%s", strings.ToLower(tc.name)))
			connID := rig.completeAuthFlow(t)

			rig.forceTokenExpired(t, connID, false)

			// Snapshot the token row AFTER force-expire (forceTokenExpired
			// itself inserts a new replacement row), so the
			// "no corrupt token state" assertion compares against the row
			// that exists when the refresh POST is about to fire. A
			// permanent refresh failure must leave this exact row in place.
			preFailureToken := rig.env.GetOAuth2Token(t, connID)
			require.NotNil(t, preFailureToken, "forge-expire must leave a token row")
			preFailureTokenID := preFailureToken.Id
			preFailureEncryptedRefresh := preFailureToken.EncryptedRefreshToken

			rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, tc.scriptAction)

			w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
			require.NotEqualf(t, 200, w.Code,
				"proxy must fail when refresh fails permanently; got 200 body=%s", w.Body.String())

			// Health flipped unhealthy — the load-bearing signal that drives
			// the marketplace reconnect prompt.
			conn := rig.env.GetConnection(t, connID)
			assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
				"permanent refresh failure must flip the connection unhealthy")

			// Exactly one refresh-failed event with the expected category
			// and status/error fields. No retries on permanent failures, so
			// the attempts field would be 1 if emitted — proxy.go omits it
			// when attempts == 1 only via the >0 gate, so we don't pin its
			// presence here; the retry tests cover attempts emission.
			failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
			require.Lenf(t, failed, 1, "expected exactly one refresh-failed event; got %d (%v)", len(failed), failed)
			event := failed[0]
			assert.Equal(t, tc.expectedCategory, event["category"], "failure category mismatch")
			assert.Equal(t, connID, event["connection_id"])
			if tc.expectedStatusCode != 0 {
				assert.Equal(t, float64(tc.expectedStatusCode), event["provider_status_code"],
					"provider_status_code mismatch for %s", tc.name)
			}
			if tc.expectedProviderErr != "" {
				assert.Equal(t, tc.expectedProviderErr, event["provider_error"],
					"provider_error mismatch for %s", tc.name)
			} else {
				_, hasProviderError := event["provider_error"]
				assert.Falsef(t, hasProviderError,
					"provider_error should be omitted when no recognized §5.2 code; got %v", event["provider_error"])
			}

			// Health-state-changed transition event with reason
			// `refresh_<category>` — this is the field dashboards join
			// against the refresh-failed event on.
			transitions := rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage)
			require.Lenf(t, transitions, 1, "expected exactly one health-state-changed event; got %d", len(transitions))
			assert.Equal(t, "healthy", transitions[0]["previous_health_state"])
			assert.Equal(t, "unhealthy", transitions[0]["health_state"])
			assert.Equal(t, "refresh_"+tc.expectedCategory, transitions[0]["reason"],
				"transition reason must encode the refresh category for dashboard correlation")

			// No refresh-succeeded event — even one would corrupt the
			// success/failure dashboards.
			assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage),
				"failed refresh must not emit a refresh-succeeded event")

			// Token state preservation. A malformed refresh response must
			// NOT overwrite the persisted refresh_token with garbage — the
			// existing row remains as forge-expired left it (same id, same
			// encrypted refresh_token bytes). If the proxy ever started
			// persisting partial refresh responses, the next refresh
			// attempt would be running with corrupted state.
			postFailureToken := rig.env.GetOAuth2Token(t, connID)
			require.NotNil(t, postFailureToken, "permanent refresh failure must not delete the token row")
			assert.Equalf(t, preFailureTokenID, postFailureToken.Id,
				"permanent refresh failure must not insert a replacement token row; new id=%s old id=%s",
				postFailureToken.Id, preFailureTokenID)
			assert.Equalf(t, preFailureEncryptedRefresh, postFailureToken.EncryptedRefreshToken,
				"permanent refresh failure must not mutate the stored refresh_token")

			// Exactly one POST to the refresh endpoint — none of the
			// scenario 7 cases are retryable, so the retry loop short-
			// circuits on the first response. completeAuthFlow uses
			// grant_type=authorization_code at the token endpoint, never
			// the refresh endpoint, so any EndpointRefresh request here
			// can only have been driven by the expired-token flow under
			// test.
			refreshCalls := 0
			for _, req := range rig.provider.Requests(helpers.RequestsFilter{
				Endpoint: helpers.EndpointRefresh,
				ClientID: rig.clientKey,
			}) {
				if lastForm(req.Form, "grant_type") == "refresh_token" {
					refreshCalls++
				}
			}
			assert.Equalf(t, 1, refreshCalls,
				"expected exactly one refresh-token POST; got %d (permanent failures must not retry)",
				refreshCalls)
		})
	}
}
