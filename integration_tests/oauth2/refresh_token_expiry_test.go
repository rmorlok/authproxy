//go:build integration

package oauth2

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// Refresh-token lifetimes (issue #1002). RFC 6749 only gives the access
// token a lifetime (expires_in); several providers and an OAuth WG draft add
// fields saying when the *refresh* token stops working. AuthProxy records
// that as refresh_token_expires_at and schedules a proactive background
// refresh (refresh_token_refresh_at) so idle connections do not lapse.
//
// Each case scripts the provider's token response in the documented shape
// of a real provider. Sources:
//
//   - GitHub Apps user tokens: refresh_token_expires_in, always 15897600
//     (6 months); refresh tokens rotate.
//     https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens
//   - LinkedIn programmatic refresh tokens: refresh_token_expires_in, 1-year
//     TTL fixed at the initial grant and counted down by later refreshes.
//     https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens
//   - Google, when the user grants time-based access: refresh_token_expires_in.
//     https://developers.google.com/identity/protocols/oauth2/web-server
//   - Intuit QuickBooks Online: x_refresh_token_expires_in (~100 days),
//     rotating. https://help.developer.intuit.com/s/article/Validity-of-Refresh-Token
//   - draft-ietf-oauth-refresh-token-expiration-03: refresh_token_timeout
//     (max time the client may hold the token without exchanging it) and
//     authorization_expires_in (consent lifetime, which refreshing cannot
//     extend, so AuthProxy ignores it for scheduling).
//     https://datatracker.ietf.org/doc/draft-ietf-oauth-refresh-token-expiration/
//
// Scripted tokens are not valid at the test provider's resource server, so
// refreshes are triggered by proxying to a local always-200 upstream rather
// than the provider's /echo.

// alwaysOKUpstream starts a local upstream that answers 200 to anything, so a
// proxied request succeeds regardless of which (scripted) access token the
// proxy applies.
func alwaysOKUpstream(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// scriptedRefreshThroughProxy scripts one successful refresh response with
// body, forces the access token expired, and proxies a request so the proxy
// refreshes. Returns the token row persisted by that refresh.
func (r *proxyRefreshRig) scriptedRefreshThroughProxy(t *testing.T, connID, upstream, body string) *database.OAuth2Token {
	t.Helper()
	r.provider.Script(r.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    body,
	})
	r.forceTokenExpired(t, connID, false)

	w := r.env.DoProxyRequest(t, connID, upstream+"/anything", "GET")
	require.Equalf(t, http.StatusOK, w.Code, "proxy must succeed after refresh; body=%s", w.Body.String())

	token := r.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token)
	return token
}

// TestRefreshTokenExpiry_ProviderResponseShapes covers each provider's field
// name and value encoding on the refresh path.
func TestRefreshTokenExpiry_ProviderResponseShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
		// lifetime is the refresh-token lifetime AuthProxy should derive.
		lifetime time.Duration
	}{
		{
			name:     "github refresh_token_expires_in",
			body:     `{"access_token":"ghu_scripted","expires_in":28800,"refresh_token":"ghr_scripted","refresh_token_expires_in":15897600,"scope":"read","token_type":"bearer"}`,
			lifetime: 15897600 * time.Second,
		},
		{
			name:     "google time-based access refresh_token_expires_in",
			body:     `{"access_token":"ya29.scripted","expires_in":3599,"refresh_token":"1//scripted","refresh_token_expires_in":604800,"scope":"read","token_type":"Bearer"}`,
			lifetime: 604800 * time.Second,
		},
		{
			name:     "intuit x_refresh_token_expires_in",
			body:     `{"token_type":"bearer","expires_in":3600,"refresh_token":"AB11scripted","x_refresh_token_expires_in":8726400,"access_token":"eyJscripted","scope":"read"}`,
			lifetime: 8726400 * time.Second,
		},
		{
			name:     "ietf draft refresh_token_timeout ignores authorization_expires_in",
			body:     `{"access_token":"a-scripted","token_type":"Bearer","expires_in":3600,"refresh_token":"r-scripted","refresh_token_timeout":604800,"authorization_expires_in":2629800,"scope":"read"}`,
			lifetime: 604800 * time.Second,
		},
		{
			name:     "lifetime as numeric string",
			body:     `{"access_token":"a-scripted","token_type":"Bearer","expires_in":3600,"refresh_token":"r-scripted","refresh_token_expires_in":"1209600","scope":"read"}`,
			lifetime: 1209600 * time.Second,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newProxyRefreshRig(t, "refresh-token-expiry")
			connID := rig.completeAuthFlow(t)
			upstream := alwaysOKUpstream(t)

			before := time.Now()
			token := rig.scriptedRefreshThroughProxy(t, connID, upstream, tc.body)

			require.NotNil(t, token.RefreshTokenExpiresAt, "refresh token expiry must be recorded")
			assert.WithinDuration(t, before.Add(tc.lifetime), *token.RefreshTokenExpiresAt, 30*time.Second)

			require.NotNil(t, token.RefreshTokenRefreshAt, "a proactive refresh must be scheduled")
			assert.WithinDuration(t,
				token.RefreshTokenExpiresAt.Add(-connectors.DefaultRefreshTimeBeforeRefreshTokenExpiry),
				*token.RefreshTokenRefreshAt, time.Second,
				"refresh is scheduled the default 24h lead before expiry")

			assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage))
		})
	}
}

// TestRefreshTokenExpiry_InitialExchangeRecordsLifetime — LinkedIn returns
// refresh_token_expires_in on the authorization-code exchange itself, so the
// lifetime must be captured before any refresh happens.
func TestRefreshTokenExpiry_InitialExchangeRecordsLifetime(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-token-expiry-initial")
	rig.provider.Script(rig.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"access_token":"AQX-initial","expires_in":5184000,"refresh_token":"AQW-initial","refresh_token_expires_in":31536000,"scope":"read"}`,
	})

	before := time.Now()
	connID := rig.completeAuthFlow(t)

	token := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token)
	require.NotNil(t, token.RefreshTokenExpiresAt)
	assert.WithinDuration(t, before.Add(31536000*time.Second), *token.RefreshTokenExpiresAt, 30*time.Second)
	require.NotNil(t, token.RefreshTokenRefreshAt)
	assert.WithinDuration(t,
		token.RefreshTokenExpiresAt.Add(-connectors.DefaultRefreshTimeBeforeRefreshTokenExpiry),
		*token.RefreshTokenRefreshAt, time.Second)
}

// TestRefreshTokenExpiry_StandardResponseRecordsNothing — a provider that
// follows RFC 6749 only (the docker test provider's real responses) leaves the
// refresh-token expiry unknown and schedules nothing.
func TestRefreshTokenExpiry_StandardResponseRecordsNothing(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-token-expiry-none")
	connID := rig.completeAuthFlow(t)

	rig.forceTokenExpired(t, connID, false)
	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	token := rig.env.GetOAuth2Token(t, connID)
	assert.Nil(t, token.RefreshTokenExpiresAt)
	assert.Nil(t, token.RefreshTokenRefreshAt)
}

// TestRefreshTokenExpiry_FixedLifetimeStopsProactiveRefresh models LinkedIn:
// the refresh token's TTL is set at the initial grant and refreshing only
// reports the remaining time. Once a refresh shows the deadline did not move,
// AuthProxy stops scheduling proactive refreshes for it — otherwise the
// background task would refresh the connection on every run until expiry.
func TestRefreshTokenExpiry_FixedLifetimeStopsProactiveRefresh(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-token-expiry-fixed")
	connID := rig.completeAuthFlow(t)
	upstream := alwaysOKUpstream(t)

	first := rig.scriptedRefreshThroughProxy(t, connID, upstream,
		`{"access_token":"AQX1","expires_in":5184000,"refresh_token":"AQW","refresh_token_expires_in":31536000,"scope":"read"}`)
	require.NotNil(t, first.RefreshTokenExpiresAt)
	require.NotNil(t, first.RefreshTokenRefreshAt)

	// Second refresh a moment later reports the same deadline (remaining
	// lifetime counted down) and returns the same refresh token.
	remaining := int(time.Until(*first.RefreshTokenExpiresAt).Seconds())
	second := rig.scriptedRefreshThroughProxy(t, connID, upstream,
		`{"access_token":"AQX2","expires_in":5184000,"refresh_token":"AQW","refresh_token_expires_in":`+strconv.Itoa(remaining)+`,"scope":"read"}`)

	require.NotNil(t, second.RefreshTokenExpiresAt)
	assert.WithinDuration(t, *first.RefreshTokenExpiresAt, *second.RefreshTokenExpiresAt, 5*time.Second,
		"fixed-lifetime providers keep the original deadline")
	assert.Nil(t, second.RefreshTokenRefreshAt,
		"a refresh that cannot extend the refresh token must not be rescheduled")
}

// TestRefreshTokenExpiry_BackgroundScanSelectsDueRefreshTokens checks the
// background refresh scan picks up a connection whose refresh token is due
// for a proactive refresh even though its access token is nowhere near
// expiry, and does not pick it up before then.
func TestRefreshTokenExpiry_BackgroundScanSelectsDueRefreshTokens(t *testing.T) {
	rig := newProxyRefreshRig(t, "refresh-token-expiry-scan")
	connID := rig.completeAuthFlow(t)
	upstream := alwaysOKUpstream(t)

	// GitHub-style rotating token with a 30-day access token so only the
	// refresh-token criterion can select it.
	token := rig.scriptedRefreshThroughProxy(t, connID, upstream,
		`{"access_token":"ghu_scan","expires_in":2592000,"refresh_token":"ghr_scan","refresh_token_expires_in":604800,"scope":"read","token_type":"bearer"}`)
	require.NotNil(t, token.RefreshTokenRefreshAt)

	connectionID, err := apid.Parse(connID)
	require.NoError(t, err)

	scanAt := func(at time.Time) bool {
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(at)).Build()
		found := false
		err := rig.env.Db.EnumerateOAuth2TokensExpiringWithin(ctx, time.Hour,
			func(tokens []*database.OAuth2TokenWithConnection, _ bool) (pagination.KeepGoing, error) {
				for _, tok := range tokens {
					if tok.Token.ConnectionId == connectionID {
						found = true
					}
				}
				return pagination.Continue, nil
			})
		require.NoError(t, err)
		return found
	}

	assert.False(t, scanAt(time.Now()), "not due yet")
	assert.False(t, scanAt(token.RefreshTokenRefreshAt.Add(-time.Minute)), "just before refresh-at")
	assert.True(t, scanAt(token.RefreshTokenRefreshAt.Add(time.Minute)), "due once refresh-at has passed")
}
