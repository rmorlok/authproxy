//go:build integration

package oauth2

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStandardAuthorizationCodeFlow walks the standard authorization-code flow
// end-to-end. See standard_flow_test.md for the scenario specification, the
// component-deployment breakdown, and a sequence diagram.
func TestStandardAuthorizationCodeFlow(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	// Use a fresh, time-based suffix per run — the test provider persists
	// clients/users across runs of `go test`, so static keys would 409 on rerun.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "standard-flow-client-" + suffix
	clientSecret := "standard-flow-secret-" + suffix
	const returnToUrl = "https://app.example.com/done"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "standard-flow-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
	})
	defer env.Cleanup()

	// Register the OAuth client at the test provider with the same redirect URI
	// the proxy will emit, so authorize matches.
	callbackURL := env.PublicOAuthCallbackURL()
	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             callbackURL,
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: "alice-" + suffix + "@example.com",
		Password: "p4ssw0rd",
		Email:    "alice-" + suffix + "@example.com",
	})

	// 1. Initiate the connection.
	connectionID, redirectURL := env.InitiateOAuth2Connection(t, connectorID, returnToUrl)
	require.NotEmpty(t, connectionID)

	// The redirect points at the public service's /oauth2/redirect with a
	// state_id and signed JWT.
	parsedRedirect, err := url.Parse(redirectURL)
	require.NoError(t, err)
	assert.Contains(t, parsedRedirect.Path, "/oauth2/redirect")
	assert.NotEmpty(t, parsedRedirect.Query().Get("state_id"))
	assert.NotEmpty(t, parsedRedirect.Query().Get("auth_token"))

	// 2. Follow the public redirect → expect Location pointing at the provider
	// authorize endpoint with the OAuth params we care about.
	providerAuthURL := env.FollowOAuth2Redirect(t, redirectURL)
	parsedAuth, err := url.Parse(providerAuthURL)
	require.NoError(t, err)
	authQ := parsedAuth.Query()

	// Spec #1 — required authorize-URL params.
	assert.Equal(t, clientKey, authQ.Get("client_id"))
	assert.Equal(t, "code", authQ.Get("response_type"))
	assert.Equal(t, callbackURL, authQ.Get("redirect_uri"))
	assert.Equal(t, "read", authQ.Get("scope"))
	stateParam := authQ.Get("state")
	assert.NotEmpty(t, stateParam, "state must be present so the callback can correlate")

	// 3. Drive provider authorize → user approves.
	authResp := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    clientKey,
		UserID:      user.ID,
		RedirectURI: callbackURL,
		Scope:       "read",
		State:       stateParam,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)

	parsedCallback, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	cbQ := parsedCallback.Query()
	require.NotEmpty(t, cbQ.Get("code"), "approve must include code")
	assert.Equal(t, stateParam, cbQ.Get("state"), "state must round-trip from authorize to callback")

	// 4. Deliver the callback to the proxy. The proxy exchanges the code for
	// tokens, persists them, and redirects the user back to return_to_url.
	finalURL := env.DeliverOAuth2Callback(t, authResp.RedirectURL)
	assert.True(t, strings.HasPrefix(finalURL, returnToUrl),
		"final redirect should target return_to_url, got %q", finalURL)

	// 5. Token persisted with non-empty encrypted material and a future expiry.
	token := env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token, "OAuth token row must be persisted after a successful callback")
	assert.False(t, token.EncryptedAccessToken.IsZero(), "access token must be stored encrypted")
	assert.False(t, token.EncryptedRefreshToken.IsZero(), "refresh token returned by provider must be stored")
	require.NotNil(t, token.AccessTokenExpiresAt, "expiry must be parsed from token response")
	assert.True(t, token.AccessTokenExpiresAt.After(time.Now()),
		"stored access token should not yet be expired (expires_at=%s)", token.AccessTokenExpiresAt)

	// 6. Connection lands in `ready` (no probes/configure on this connector).
	conn := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State,
		"connection should be ready after a successful flow with no probes/configure steps")

	// 7. Proxy a request to the provider's resource endpoint and expect 200.
	w := env.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code, "proxy endpoint should return 200; body=%s", w.Body.String())

	// 8. The provider must have observed both the token exchange and the
	// proxied request, and the proxied request must have carried a Bearer.
	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	require.NotEmpty(t, tokenReqs, "provider should have recorded a /token call")
	assert.Equal(t, "authorization_code", lastForm(tokenReqs[len(tokenReqs)-1].Form, "grant_type"))

	resourceReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointResource,
	})
	require.NotEmpty(t, resourceReqs, "provider should have recorded the proxied API call")
	authHeader := resourceReqs[len(resourceReqs)-1].Headers["Authorization"]
	if authHeader == "" {
		authHeader = resourceReqs[len(resourceReqs)-1].Headers["authorization"]
	}
	require.NotEmpty(t, authHeader, "proxied request must include Authorization")
	assert.True(t, strings.HasPrefix(strings.ToLower(authHeader), "bearer "),
		"proxied request must use Bearer scheme, got %q", authHeader)
}

func lastForm(form map[string][]string, key string) string {
	v := form[key]
	if len(v) == 0 {
		return ""
	}
	return v[len(v)-1]
}
