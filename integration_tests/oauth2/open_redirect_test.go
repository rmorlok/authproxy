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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuth2OpenRedirectProtection_InvalidReturnURLFallsBackToMarketplace(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "open-redirect-client-" + suffix
	clientSecret := "open-redirect-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := "open-redirect-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "open-redirect-test", provider, helpers.OAuth2ConnectorOptions{
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
	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	maliciousReturnURL := "https://evil.example/phish"
	_, redirectURL := env.InitiateOAuth2Connection(t, connectorID, maliciousReturnURL)

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(redirectURL),
		chromedp.WaitVisible(`input[name="email"]`, chromedp.ByQuery),
	))
	require.NotEmpty(t, redirectURL)

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="allow"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))

	var finalURL string
	require.NoError(t, chromedp.Run(browserCtx, chromedp.Location(&finalURL)))
	assert.Truef(t, strings.HasPrefix(finalURL, env.PublicURL+"/connections"),
		"invalid return_to_url should fall back to marketplace connections page; got %q", finalURL)
	assert.NotContains(t, finalURL, "evil.example", "callback must not redirect to the attacker-controlled origin")

	page := env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Len(t, page.Results, 1)
	assert.Equal(t, database.ConnectionStateConfigured, page.Results[0].State)
	require.NotNil(t, env.GetOAuth2Token(t, page.Results[0].Id.String()))
}
