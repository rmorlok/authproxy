//go:build integration

package oauth2

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStandardAuthorizationCodeFlow walks the standard authorization-code flow
// end-to-end through real UIs: a headless Chrome opens the marketplace SPA,
// clicks Connect on the connector card, the proxy redirects to the upstream
// provider, the user logs in and approves the consent screen, and the proxy
// stores tokens and lands the browser back on the return-to URL.
//
// See standard_flow_test.md for the scenario specification, component
// breakdown, and sequence diagram.
func TestStandardAuthorizationCodeFlow(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	// Use a fresh, time-based suffix per run — the test provider persists
	// clients/users across runs of `go test`, so static keys would 409 on rerun.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "standard-flow-client-" + suffix
	clientSecret := "standard-flow-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := "alice-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "standard-flow-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:            helpers.ServiceTypeAPI,
		StartHTTPServer:    true,
		IncludePublic:      true,
		ServeMarketplaceUI: true,
		Connectors:         []sconfig.Connector{connector},
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
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	// Sign a marketplace auth_token for the test actor. The SPA's
	// session._initiate exchanges this for a SESSION-ID cookie on first load.
	authToken, err := env.PublicAuthUtil.GenerateBearerToken(
		context.Background(),
		"test-actor",
		sconfig.RootNamespace,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	// 1. Open the marketplace, signed in as the test actor.
	browserCtx, _ := helpers.NewBrowser(t)

	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		// The Connect button only renders after the SPA's session._initiate
		// resolves and the connectors API returns the test connector.
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	// 2. Click Connect — the SPA initiates the connection and navigates to the
	//    public service's /oauth2/redirect, which 302s to the provider.
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Connect']`, chromedp.BySearch),
		// Wait until we've left the marketplace; the redirect chain ends on
		// the provider's /web/login form.
		chromedp.WaitVisible(`input[name="email"]`, chromedp.ByQuery),
	))

	// 3. Log in as the test user.
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
		// Login redirects to /web/authorize, which renders the consent form.
		chromedp.WaitVisible(`input[name="allow"]`, chromedp.ByQuery),
	))

	// 4. Click Allow. The provider redirects back to the proxy's callback,
	//    which exchanges the code for tokens and lands the browser on
	//    return_to_url (the marketplace's /connections page). Wait for the
	//    "Your Connections" heading rendered by ConnectionList so we don't
	//    inspect the URL mid-navigation.
	expectedReturnPrefix := env.PublicURL + "/connections"
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))

	var finalURL string
	require.NoError(t, chromedp.Run(browserCtx, chromedp.Location(&finalURL)))
	assert.Truef(t, strings.HasPrefix(finalURL, expectedReturnPrefix),
		"expected to land on return_to_url (%s), got %q", expectedReturnPrefix, finalURL)

	// 5. Locate the new connection. The marketplace creates one new connection
	//    per Connect click, so we list and pick the only one.
	page := env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Lenf(t, page.Results, 1, "expected exactly one connection after Connect; got %d", len(page.Results))
	connectionID := page.Results[0].Id.String()

	// 6. Token persisted with non-empty encrypted material and a future expiry.
	token := env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token, "OAuth token row must be persisted after a successful callback")
	assert.False(t, token.EncryptedAccessToken.IsZero(), "access token must be stored encrypted")
	assert.False(t, token.EncryptedRefreshToken.IsZero(), "refresh token returned by provider must be stored")
	require.NotNil(t, token.AccessTokenExpiresAt, "expiry must be parsed from token response")
	assert.True(t, token.AccessTokenExpiresAt.After(time.Now()),
		"stored access token should not yet be expired (expires_at=%s)", token.AccessTokenExpiresAt)

	// 7. Connection lands in `ready` (no probes/configure on this connector).
	conn := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State,
		"connection should be ready after a successful flow with no probes/configure steps")

	// 8. Proxy a request to the provider's resource endpoint and expect 200.
	w := env.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code, "proxy endpoint should return 200; body=%s", w.Body.String())

	// 9. The provider must have observed both the token exchange and the
	//    proxied request, and the proxied request must have carried a Bearer.
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
