//go:build integration

package oauth2

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These constants mirror the production strings from
// internal/auth_methods/oauth2 and internal/core. The integration tests
// live in a different package so we redeclare here; if either string ever
// drifts, the test fails fast on a missing record rather than silently
// passing because no events match.
const (
	tokenRefreshSuccessMessage          = "oauth token refresh succeeded"
	tokenRefreshFailureMessage          = "oauth token refresh failed"
	connectionHealthStateChangedMessage = "connection health state changed"
)

// proxyRefreshRig holds a fully-wired connection that has completed the
// standard auth flow and persisted real provider tokens. Tests then
// `forceTokenExpired` to mutate the persisted access-token expiry into
// the past so the next proxy call exercises the refresh path.
//
// We use the /test/authorize shortcut (not chromedp) per #169's note: the
// scenarios under test are about token-endpoint behavior and timing, not
// the user-facing consent leg.
type proxyRefreshRig struct {
	provider    *helpers.OAuth2TestProvider
	env         *helpers.IntegrationTestEnv
	logCapture  *helpers.LogCapture
	clientKey   string
	userID      string
	connectorID apid.ID
	scopes      []string
	returnToURL string
}

func newProxyRefreshRig(t *testing.T, name string) *proxyRefreshRig {
	t.Helper()
	provider := helpers.NewOAuth2TestProvider(t)
	provider.SetRefreshRotation(true)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
	})
	t.Cleanup(env.Cleanup)

	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: "p4ssw0rd-" + suffix,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &proxyRefreshRig{
		provider:    provider,
		env:         env,
		logCapture:  logCapture,
		clientKey:   clientKey,
		userID:      user.ID,
		connectorID: connectorID,
		scopes:      []string{"read"},
		returnToURL: env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}
}

// completeAuthFlow drives the standard authorization-code flow to a Ready
// connection with real provider-issued tokens persisted. Same pattern as
// callback_token_exchange_retry_test.go's initiateAndMintCode + callback
// delivery, just packaged as a one-call helper since refresh tests never
// care about intermediate state.
func (r *proxyRefreshRig) completeAuthFlow(t *testing.T) string {
	t.Helper()
	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(r.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	pc, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := pc.Query().Get("code")
	require.NotEmpty(t, code)

	loc := r.env.DeliverOAuth2Callback(t, r.env.ForgeOAuth2CallbackURL(stateID, code))
	require.Truef(t, strings.HasPrefix(loc, r.returnToURL),
		"auth flow should land on return_to_url; got %q", loc)
	return connID
}

// forceTokenExpired replaces the persisted token row with one whose
// AccessTokenExpiresAt is in the past, optionally clearing the
// refresh_token. The existing encrypted fields are reused as-is — we
// don't need to re-encrypt because the proxy's expiry check fires
// before any decrypt, and (for the refresh-token-present case) the
// underlying provider-issued refresh_token is still valid.
func (r *proxyRefreshRig) forceTokenExpired(t *testing.T, connectionID string, clearRefreshToken bool) {
	t.Helper()
	r.forceTokenExpiresAt(t, connectionID, time.Now().Add(-1*time.Hour), clearRefreshToken)
}

func (r *proxyRefreshRig) forceTokenExpiresAt(t *testing.T, connectionID string, expiresAt time.Time, clearRefreshToken bool) {
	t.Helper()
	ctx := context.Background()
	existing := r.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, existing, "auth flow must have persisted a token")
	require.False(t, existing.EncryptedRefreshToken.IsZero(),
		"fixture invariant: provider must have issued a refresh_token in the initial grant")
	require.False(t, existing.EncryptedAccessToken.IsZero(), "fixture invariant: access_token present")

	refreshToken := existing.EncryptedRefreshToken
	if clearRefreshToken {
		refreshToken = encfield.EncryptedField{}
	}

	connID, err := apid.Parse(connectionID)
	require.NoError(t, err)
	_, err = r.env.Db.InsertOAuth2Token(
		ctx,
		connID,
		nil,
		refreshToken,
		existing.EncryptedAccessToken,
		&expiresAt,
		existing.Scopes,
		existing.RequestedScopes,
		existing.CreatedByActorId,
	)
	require.NoError(t, err, "force-expiry: insert replacement token row")

	// Sanity: GetOAuth2Token now returns the forged row.
	current := r.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, current)
	require.NotNil(t, current.AccessTokenExpiresAt)
	require.WithinDuration(t, expiresAt, *current.AccessTokenExpiresAt, time.Second)
}

// TestProxyRefresh_ExpiredAccessTokenRefreshes covers scenario 6 from #169:
// the proxy detects an expired access token at request time, uses the
// refresh_token to obtain a new one from the provider, persists it, and
// the original request succeeds. The connection stays healthy throughout —
// a successful refresh is invisible to the customer's app modulo the
// fresh token.
func TestProxyRefresh_ExpiredAccessTokenRefreshes(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-refresh-success")
	connID := rig.completeAuthFlow(t)

	original := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, original)
	originalTokenID := original.Id

	rig.forceTokenExpired(t, connID, false)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code,
		"proxy must succeed after refreshing the expired token; got %d body=%s", w.Code, w.Body.String())

	// Refreshed-token assertions: new row, future expiry, refreshedFrom
	// pointer chained to the prior token id (so audit/lineage works).
	refreshed := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, refreshed)
	assert.NotEqualf(t, originalTokenID, refreshed.Id, "refresh must persist a new token row")
	require.NotNil(t, refreshed.AccessTokenExpiresAt)
	assert.Truef(t, refreshed.AccessTokenExpiresAt.After(time.Now()),
		"refreshed access_token must have a future expiry; got %s", refreshed.AccessTokenExpiresAt)

	// Exactly one refresh-token grant observed at the provider — the
	// retry-once-after-refresh path in ProxyRequest should NOT have fired
	// here because the proactive expiry check refreshes before the 401
	// can happen. The test provider categorizes refresh-token grants under
	// EndpointRefresh (not EndpointToken, which is reserved for the initial
	// authorization_code exchange).
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
		"expected exactly one grant_type=refresh_token call; got %d", refreshCalls)

	// Structured success event with the right connection_id.
	succeeded := rig.logCapture.RecordsWithMessage(t, tokenRefreshSuccessMessage)
	require.Lenf(t, succeeded, 1, "expected exactly one refresh-succeeded event; got %d", len(succeeded))
	assert.Equal(t, connID, succeeded[0]["connection_id"])

	// No refresh-failed event — even one would corrupt dashboards.
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"successful refresh must not emit a refresh-failed event")

	// Connection health stays healthy. MarkHealthState is idempotent, so
	// a successful refresh on an already-healthy connection should NOT
	// emit a "connection health state changed" transition event.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateHealthy, conn.HealthState)
	assert.Empty(t, rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage),
		"idempotent healthy→healthy must not emit a transition event")
}

func TestProxyRefresh_NearlyExpiredAccessTokenRefreshesWithinBuffer(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-refresh-near-expiry")
	connID := rig.completeAuthFlow(t)

	rig.forceTokenExpiresAt(t, connID, time.Now().Add(database.OAuth2AccessTokenExpiryBuffer-time.Second), false)
	forged := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, forged)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code,
		"proxy must refresh a token inside the expiry buffer; got %d body=%s", w.Code, w.Body.String())

	refreshed := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, refreshed)
	assert.NotEqual(t, forged.Id, refreshed.Id, "near-expiry token should be replaced by refresh")
	require.Len(t, refreshGrantRequests(rig), 1, "token inside expiry buffer should refresh exactly once")
}

func TestProxyRefresh_TokenOutsideExpiryBufferDoesNotRefreshAggressively(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-refresh-outside-buffer")
	connID := rig.completeAuthFlow(t)

	rig.forceTokenExpiresAt(t, connID, time.Now().Add(database.OAuth2AccessTokenExpiryBuffer+10*time.Second), false)
	forged := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, forged)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code,
		"proxy should use token outside the expiry buffer; got %d body=%s", w.Code, w.Body.String())

	current := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, current)
	assert.Equal(t, forged.Id, current.Id, "token outside expiry buffer should not be refreshed")
	require.Empty(t, refreshGrantRequests(rig), "token outside expiry buffer should not trigger refresh")
}

// TestProxyRefresh_NoRefreshTokenFlipsUnhealthy covers scenario 13:
// the connection was established with no refresh_token (provider issued
// an access-token-only grant). After the access token expires, the
// proxy cannot obtain a new one without user interaction, so it must
// flip the connection's health_state to unhealthy — that's the signal
// the marketplace keys off to render the reconnect prompt.
//
// Critical invariants: (1) no HTTP call to /token is made (the
// no-refresh-token category short-circuits before the POST), (2) the
// proxy request itself fails (no token = no proxy), (3) the health
// transition event records reason=refresh_no_refresh_token so
// dashboards can correlate this with refresh-failure events.
func TestProxyRefresh_NoRefreshTokenFlipsUnhealthy(t *testing.T) {
	rig := newProxyRefreshRig(t, "proxy-refresh-no-refresh-token")
	connID := rig.completeAuthFlow(t)

	refreshCallsBefore := len(rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRefresh,
		ClientID: rig.clientKey,
	}))

	rig.forceTokenExpired(t, connID, true)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.NotEqualf(t, 200, w.Code,
		"proxy must fail when access token is expired and no refresh_token exists; got 200 body=%s", w.Body.String())

	// Health flipped to unhealthy — the load-bearing signal.
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionHealthStateUnhealthy, conn.HealthState,
		"no-refresh-token expiry must flip the connection unhealthy")

	// Structured refresh-failed event with category=no_refresh_token.
	failed := rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage)
	require.Lenf(t, failed, 1, "expected exactly one refresh-failed event; got %d", len(failed))
	assert.Equal(t, "no_refresh_token", failed[0]["category"])
	assert.Equal(t, connID, failed[0]["connection_id"])

	// Health-state-changed event with reason=refresh_no_refresh_token.
	// This is what dashboards correlate with the refresh-failure event.
	transitions := rig.logCapture.RecordsWithMessage(t, connectionHealthStateChangedMessage)
	require.Lenf(t, transitions, 1, "expected exactly one health-state-changed event; got %d", len(transitions))
	assert.Equal(t, "healthy", transitions[0]["previous_health_state"])
	assert.Equal(t, "unhealthy", transitions[0]["health_state"])
	assert.Equal(t, "refresh_no_refresh_token", transitions[0]["reason"])

	// No HTTP call to the refresh endpoint after the auth flow finished.
	// The no-refresh-token short-circuit must happen before any POST so we
	// don't waste a provider round-trip (and don't risk a spurious
	// invalid_request from the empty refresh_token field).
	refreshCallsAfter := len(rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRefresh,
		ClientID: rig.clientKey,
	}))
	assert.Equalf(t, refreshCallsBefore, refreshCallsAfter,
		"no-refresh-token path must not POST a refresh-token grant (before=%d, after=%d)",
		refreshCallsBefore, refreshCallsAfter)
}
