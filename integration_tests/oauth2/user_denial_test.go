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
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserDenialFlow walks the user-denial branch of the standard
// authorization-code flow end-to-end through real UIs: a headless Chrome opens
// the marketplace SPA, clicks Connect, logs in at the provider, and then
// clicks Deny on the consent screen. The provider redirects back to the
// proxy's callback with `error=access_denied`, the proxy records the denial
// on the connection, and the browser lands on the marketplace's connections
// page.
//
// See user_denial_test.md for the scenario specification and assertions.
func TestUserDenialFlow(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	// Fresh, time-based suffix per run — the provider persists clients/users
	// across runs of `go test`.
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "user-denial-client-" + suffix
	clientSecret := "user-denial-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := "deny-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "user-denial-test", provider, helpers.OAuth2ConnectorOptions{
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

	authToken, err := env.PublicAuthUtil.GenerateBearerToken(
		context.Background(),
		"test-actor",
		sconfig.RootNamespace,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)

	// 1. Marketplace boot + Connect click — same as the happy path up through
	//    the provider's consent screen.
	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Connect']`, chromedp.BySearch),
		chromedp.WaitVisible(`input[name="email"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
		// Login redirects to /web/authorize, which renders the consent form.
		// Wait for the Deny button specifically — the form has both Allow and
		// Deny submit buttons (see go-oauth2-server web/includes/authorize.html).
		chromedp.WaitVisible(`input[name="deny"]`, chromedp.ByQuery),
	))

	// 2. Click Deny. Provider redirects to /oauth2/callback?error=access_denied&state=...
	//    The proxy records the denial and redirects back to the return URL
	//    with setup=pending so the marketplace can render the failure state.
	expectedReturnPrefix := env.PublicURL + "/connections"
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="deny"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))

	var finalURL string
	require.NoError(t, chromedp.Run(browserCtx, chromedp.Location(&finalURL)))
	assert.Truef(t, strings.HasPrefix(finalURL, expectedReturnPrefix),
		"expected to land on return_to_url (%s), got %q", expectedReturnPrefix, finalURL)
	// The proxy redirects with ?setup=pending&connection_id=... but the SPA
	// strips those params from the URL as soon as it consumes them
	// (ui/marketplace/src/components/ConnectionList.tsx). Reading Location()
	// here would race that cleanup, so we don't assert the suffix — the
	// connection-level checks below prove the denial path executed.

	// 3. Locate the connection. The marketplace creates one connection per
	//    Connect click; on denial it stays around in auth_failed for retry.
	page := env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Lenf(t, page.Results, 1, "expected exactly one connection after Connect; got %d", len(page.Results))
	connectionID := page.Results[0].Id.String()

	// 4. No token row was persisted — denial means no exchange.
	token := env.GetOAuth2Token(t, connectionID)
	assert.Nil(t, token, "no OAuth token row should exist when the user denied authorization")

	// 5. Connection sits in the auth_failed setup phase, retryable. State stays
	//    at `created` because we never reached HandleCredentialsEstablished.
	conn := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateCreated, conn.State,
		"connection should remain in created state when authorization was denied")
	require.NotNilf(t, conn.SetupStep, "denied connection should have a setup_step recorded")
	assert.Truef(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"connection should be in auth_failed setup step after denial; got %q", conn.SetupStep.String())
	require.NotNilf(t, conn.SetupError, "denied connection should have setup_error recorded")
	assert.Containsf(t, *conn.SetupError, "access_denied",
		"setup_error should reflect the provider's access_denied code; got %q", *conn.SetupError)

	// 6. Provider observed no /token call — denial means no code-for-token
	//    exchange should have been attempted.
	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when the user denied authorization")
}
