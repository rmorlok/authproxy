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

// TestCallbackRejection_CrossNamespace covers issue #167 case 6: a multi-tenant
// AuthProxy deployment where two customer apps share the instance and use
// child namespaces (`root.tenant-a`, `root.tenant-b`) to isolate their actors.
// The same external_id can refer to two different users — one in each
// namespace — because actor rows are scoped per-namespace.
//
// Threat: an attacker (alice in tenant-a) initiates a connection, drives the
// provider's authorize step to mint a code, and sends the resulting callback
// URL to a victim with the same external_id in a different tenant (bob in
// tenant-b). When bob's browser follows the link, the public service
// identifies bob from his SESSION-ID cookie. Because actor IDs are
// independently allocated per (namespace, external_id), bob's actor id
// differs from alice's — so state validation rejects with `actor_mismatch`
// before reaching the namespace-specific defense-in-depth checks.
//
// The two `namespace_mismatch_*` categories are tested separately in
// callback_namespace_mismatch_test.go via direct state-envelope injection.
func TestCallbackRejection_CrossNamespace(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantA := "root.tenant-a-" + suffix
	tenantB := "root.tenant-b-" + suffix
	// Same external_id in both tenants — the multi-tenant collision the
	// test exercises. Each tenant's actor row gets its own actor_id.
	sharedExternalID := "user-123-" + suffix
	clientKey := "cross-ns-client-" + suffix
	clientSecret := "cross-ns-secret-" + suffix
	providerUserPassword := "p4ssw0rd-" + suffix
	providerUserEmail := "alice-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "cross-ns-test", provider, helpers.OAuth2ConnectorOptions{
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

	ctx := context.Background()
	_, err := env.Core.CreateNamespace(ctx, tenantA, nil)
	require.NoError(t, err)
	_, err = env.Core.CreateNamespace(ctx, tenantB, nil)
	require.NoError(t, err)

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

	// 1. Attacker initiates as alice in tenant-a. State stored with
	//    Namespace=tenant-a, ActorId=<alice's tenant-a actor>.
	returnTo := "https://example.com/return"
	connID, redirectURL := env.InitiateOAuth2Connection(t, connectorID, returnTo, helpers.WithActor(sharedExternalID, tenantA))
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "InitiateOAuth2ConnectionAsActor should embed state_id: %s", redirectURL)

	// 2. Mint a code via the test provider's /test/authorize. The provider
	//    only validates against its own client/user records, so the
	//    attacker can drive this leg programmatically.
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

	// 3. Victim bob's marketplace session in tenant-b. Same external_id
	//    as alice but a different namespace, so the auth middleware
	//    materializes a separate actor row with a different actor_id.
	bobAuthToken, err := env.PublicAuthUtil.GenerateBearerToken(
		ctx, sharedExternalID, tenantB, aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)

	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(bobAuthToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	// 4. Bob's browser follows the forged callback link. The browser
	//    carries bob's SESSION-ID cookie, so the public service
	//    identifies bob (tenant-b actor); state validation finds
	//    s.ActorId (alice in tenant-a) != caller.Id (bob in tenant-b)
	//    and rejects with actor_mismatch — the namespace-specific checks
	//    never run because actor identity is checked first.
	forgedURL := env.PublicURL + "/oauth2/callback?state=" + url.QueryEscape(stateID) + "&code=" + url.QueryEscape(code)
	errorPageURL := env.Cfg.GetRoot().ErrorPages.InternalError
	require.NotEmpty(t, errorPageURL, "test config must set error_pages.internal_error")

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(forgedURL),
		chromedp.WaitVisible(`h1`, chromedp.ByQuery),
	))

	var finalURL string
	require.NoError(t, chromedp.Run(browserCtx, chromedp.Location(&finalURL)))
	assert.Equalf(t, errorPageURL, finalURL,
		"bob's browser should land on error_pages.internal_error after rejection; got %q", finalURL)

	// 5. Exactly one rejection event with category=actor_mismatch.
	events := logCapture.RecordsWithMessage(t, rejectionEventMessage)
	require.Lenf(t, events, 1, "expected exactly one rejection event; got %d (%v)", len(events), events)
	assert.Equal(t, "actor_mismatch", events[0]["category"],
		"cross-namespace attack rejects on actor_mismatch first; namespace_mismatch_actor is defense-in-depth tested separately")
	assert.Equal(t, stateID, events[0]["state_id"], "rejection event should record the state_id")

	// 6. No credentials attached to bob's tenant. alice's connection in
	//    tenant-a still has no token; bob in tenant-b owns no connection
	//    related to this flow.
	require.Nil(t, env.GetOAuth2Token(t, connID), "no oauth2_token row should exist for the rejected callback")

	conn := env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateCreated, conn.State,
		"connection state should remain `created` after a rejected callback")
	assert.Nil(t, conn.SetupStep, "no setup_step should be recorded on a rejected callback")
	assert.Nil(t, conn.SetupError, "no setup_error should be recorded on a rejected callback")
	assert.Equal(t, tenantA, conn.Namespace, "connection still belongs to alice's tenant")

	// 7. Provider observed zero /token calls.
	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when callback rejected")
}
