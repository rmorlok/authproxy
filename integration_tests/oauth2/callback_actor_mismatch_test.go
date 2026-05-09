//go:build integration

package oauth2

import (
	"context"
	"fmt"
	"net/url"
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

// TestCallbackRejection_ActorMismatch covers issue #167 case 5: an attacker
// initiates an OAuth flow, completes the provider authorize step to mint a
// code, and sends the resulting `/oauth2/callback?state=…&code=…` URL to a
// different actor (the victim) — e.g., via a phishing link. When the victim's
// browser follows the link, the public service identifies the victim from
// their SESSION-ID cookie, but the state record carries the attacker's actor
// id. State validation must reject with `actor_mismatch` and redirect to the
// configured error page; no token must be exchanged or persisted.
//
// See callback_actor_mismatch_test.md for the scenario specification, threat
// model rationale, and sequence diagram.
func TestCallbackRejection_ActorMismatch(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	attackerExternalID := "alice-attacker-" + suffix
	victimExternalID := "bob-victim-" + suffix
	clientKey := "actor-mismatch-client-" + suffix
	clientSecret := "actor-mismatch-secret-" + suffix
	providerUserPassword := "p4ssw0rd-" + suffix
	providerUserEmail := "alice-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "actor-mismatch-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:            helpers.ServiceTypeAPI,
		StartHTTPServer:    true,
		IncludePublic:      true,
		ServeMarketplaceUI: true,
		Connectors:         []sconfig.Connector{connector},
		LogCapture:         logCapture,
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

	providerUser := provider.CreateUser(helpers.CreateUserRequest{
		Username: providerUserEmail,
		Password: providerUserPassword,
		Email:    providerUserEmail,
	})
	require.NotEmpty(t, providerUser.ID)

	// 1. Attacker initiates the connection. State stored in Redis with
	//    ActorId = attacker. The connection row gets the attacker's actor.
	returnTo := "https://example.com/return"
	connID, redirectURL := env.InitiateOAuth2Connection(t, connectorID, returnTo, helpers.WithActor(attackerExternalID, sconfig.RootNamespace))
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "InitiateOAuth2ConnectionAsActor should embed state_id in redirect URL: %s", redirectURL)

	// 2. Mint a code via the test provider's /test/authorize. The provider
	//    doesn't care which proxy actor owns the state — it's validating
	//    against its own client/user records — so the attacker can drive
	//    this leg programmatically. The output is the exact callback URL
	//    the provider would have produced after a real consent.
	authResp := provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    clientKey,
		UserID:      providerUser.ID,
		RedirectURI: callbackURL,
		Scope:       "read",
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)
	providerCallback, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := providerCallback.Query().Get("code")
	require.NotEmpty(t, code, "provider should issue a code on approve; got %s", authResp.RedirectURL)

	// 3. Victim's marketplace session: open chromedp, navigate to
	//    /connectors?auth_token=<victim>. The SPA bootstrap calls
	//    /session/_initiate, which exchanges the JWT for a SESSION-ID
	//    cookie scoped to the victim. We wait for the Connect button as
	//    the bootstrap-complete signal — same marker standard_flow_test
	//    uses.
	victimAuthToken, err := env.PublicAuthUtil.GenerateBearerToken(
		context.Background(), victimExternalID, sconfig.RootNamespace, aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)

	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(victimAuthToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	// 4. Victim's browser follows the forged callback link. The browser
	//    carries the victim's SESSION-ID cookie, so the public service
	//    identifies the victim as the calling actor; state validation
	//    detects ActorId mismatch and redirects to the error page.
	forgedURL := env.PublicURL + "/oauth2/callback?state=" + url.QueryEscape(stateID) + "&code=" + url.QueryEscape(code)
	errorPageURL := env.Cfg.GetRoot().ErrorPages.InternalError
	require.NotEmpty(t, errorPageURL, "test config must set error_pages.internal_error")

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(forgedURL),
		// example.com renders <h1>Example Domain</h1> reliably; we use
		// it as a load signal after the proxy's 302.
		chromedp.WaitVisible(`h1`, chromedp.ByQuery),
	))

	var finalURL string
	require.NoError(t, chromedp.Run(browserCtx, chromedp.Location(&finalURL)))
	assert.Equalf(t, errorPageURL, finalURL,
		"victim's browser should land on error_pages.internal_error after actor_mismatch rejection; got %q", finalURL)

	// 5. Exactly one rejection event with category=actor_mismatch, carrying
	//    the state id. The actor_id field reflects the calling actor (the
	//    victim) so SOC analysts see who was hit by the link.
	events := logCapture.RecordsWithMessage(t, rejectionEventMessage)
	require.Lenf(t, events, 1, "expected exactly one rejection event; got %d (%v)", len(events), events)
	assert.Equal(t, "actor_mismatch", events[0]["category"], "rejection category mismatch")
	assert.Equal(t, stateID, events[0]["state_id"], "rejection event should record the state_id")

	// 6. No token row was written.
	require.Nil(t, env.GetOAuth2Token(t, connID), "no oauth2_token row should exist after actor_mismatch rejection")

	// 7. Connection still in `created`, no setup_step transition.
	conn := env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateCreated, conn.State,
		"connection state should remain `created` after a rejected callback")
	assert.Nil(t, conn.SetupStep, "no setup_step should be recorded on a rejected callback")
	assert.Nil(t, conn.SetupError, "no setup_error should be recorded on a rejected callback")

	// 8. Provider observed zero /token calls — the token exchange path was
	//    short-circuited by state validation.
	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when actor_mismatch rejected")
}
