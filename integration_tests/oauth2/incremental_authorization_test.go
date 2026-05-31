//go:build integration

package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Scenario 25 from issue #180: user-driven OAuth reauth can upgrade granted
// scopes. The key invariants are that the existing connection remains usable
// while the upgrade is pending, a failed upgrade does not replace working
// credentials, and a successful upgrade persists the expanded granted scope set.

type incrementalAuthRig struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	clientKey    string
	clientSecret string
	userEmail    string
	userPassword string
	connectorID  apid.ID
	resourcePath string
}

func newIncrementalAuthRig(t *testing.T, name string) *incrementalAuthRig {
	t.Helper()

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, helpers.OAuth2ConnectorOptions{
		ClientID:       clientKey,
		ClientSecret:   clientSecret,
		OptionalScopes: []string{"read_write"},
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:            helpers.ServiceTypeAPI,
		StartHTTPServer:    true,
		IncludePublic:      true,
		ServeMarketplaceUI: true,
		Connectors:         []sconfig.Connector{connector},
	})
	t.Cleanup(env.Cleanup)

	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read_write",
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &incrementalAuthRig{
		provider:     provider,
		env:          env,
		clientKey:    clientKey,
		clientSecret: clientSecret,
		userEmail:    userEmail,
		userPassword: userPassword,
		connectorID:  connectorID,
		resourcePath: "/echo",
	}
}

func (r *incrementalAuthRig) startBrowser(t *testing.T) context.Context {
	t.Helper()

	authToken, err := r.env.PublicAuthUtil.GenerateBearerToken(
		context.Background(),
		"test-actor",
		sconfig.RootNamespace,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)
	connectorsURL := r.env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))
	return browserCtx
}

func (r *incrementalAuthRig) connectReadOnly(t *testing.T, browserCtx context.Context) string {
	t.Helper()

	r.provider.Script(r.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read"),
	})

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))
	require.NoError(t, waitVisibleOrDump(t, browserCtx, `input[name="email"]`, chromedp.ByQuery))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, r.userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, r.userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
	))
	require.NoError(t, waitVisibleOrDump(t, browserCtx, `input[name="allow"]`, chromedp.ByQuery))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
	))
	require.NoError(t, waitVisibleOrDump(t, browserCtx, `//h1[normalize-space()='Your Connections']`, chromedp.BySearch))

	page := r.env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Lenf(t, page.Results, 1, "expected exactly one connection after Connect; got %d", len(page.Results))
	return page.Results[0].Id.String()
}

func (r *incrementalAuthRig) startReauthAndWaitForConsent(t *testing.T, browserCtx context.Context, scopeOverride *string, failure *helpers.ScriptAction) {
	t.Helper()
	if scopeOverride != nil {
		r.provider.Script(r.clientKey, helpers.EndpointToken, helpers.ScriptAction{
			ScopeOverride: scopeOverride,
		})
	}
	if failure != nil {
		r.provider.Script(r.clientKey, helpers.EndpointToken, *failure)
	}

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Re-authenticate']`, chromedp.BySearch),
	))
	require.NoError(t, waitVisibleOrDump(t, browserCtx, `input[name="allow"]`, chromedp.ByQuery))
}

func (r *incrementalAuthRig) approveReauth(t *testing.T, browserCtx context.Context) {
	t.Helper()
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))
}

func (r *incrementalAuthRig) proxyResourceStatus(t *testing.T, connectionID, path string) int {
	t.Helper()
	w := r.env.DoProxyRequest(t, connectionID, r.provider.ResourceURL(path), http.MethodGet)
	resp := parseRevocationProxyResponse(t, w)
	return resp.StatusCode
}

func (r *incrementalAuthRig) requireTokenScopes(t *testing.T, connectionID, wantGranted string) string {
	t.Helper()
	token := r.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token)
	assert.Equal(t, wantGranted, token.Scopes)
	assert.Equal(t, "read_write", token.RequestedScopes)
	return token.Id.String()
}

func (r *incrementalAuthRig) requireConnectionScopes(t *testing.T, connectionID string, wantRequested, wantGranted []string) {
	t.Helper()

	path := "/api/v1/connections/" + connectionID + "/scopes"
	req, err := r.env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		path,
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	abs, err := url.Parse(r.env.ServerURL + path)
	require.NoError(t, err)
	req.URL = abs
	req.Host = abs.Host
	req.RequestURI = ""

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "scopes endpoint failed: %s", string(body))

	var out map[string][]string
	require.NoErrorf(t, json.Unmarshal(body, &out), "decode scopes body: %s", string(body))
	assert.ElementsMatch(t, wantRequested, out["requested"])
	assert.ElementsMatch(t, wantGranted, out["granted"])
}

func waitVisibleOrDump(t *testing.T, ctx context.Context, sel any, opts ...chromedp.QueryOption) error {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := chromedp.Run(waitCtx, chromedp.WaitVisible(sel, opts...)); err != nil {
		var location, body string
		_ = chromedp.Run(ctx,
			chromedp.Location(&location),
			chromedp.OuterHTML("body", &body, chromedp.ByQuery),
		)
		t.Logf("timed out waiting for %v at %s; body=%s", sel, location, body)
		return err
	}
	return nil
}

func TestIncrementalAuthorization_ReauthUpgradesScopes(t *testing.T) {
	rig := newIncrementalAuthRig(t, "incremental-success")
	browserCtx := rig.startBrowser(t)
	connID := rig.connectReadOnly(t, browserCtx)

	originalTokenID := rig.requireTokenScopes(t, connID, "read")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath))

	rig.startReauthAndWaitForConsent(t, browserCtx, ptr("read_write"), nil)

	connDuring := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, connDuring.State)
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath),
		"existing credential should remain usable while upgrade consent is pending")
	assert.Equal(t, originalTokenID, rig.requireTokenScopes(t, connID, "read"),
		"pending upgrade must not replace the stored token")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})

	rig.approveReauth(t, browserCtx)

	newTokenID := rig.requireTokenScopes(t, connID, "read_write")
	assert.NotEqual(t, originalTokenID, newTokenID,
		"successful incremental auth should persist a replacement token row")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read_write"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath))

	connAfter := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, connAfter.State)
	assert.Nil(t, connAfter.SetupStep, "successful reauth should clear setup_step")
	assert.Nil(t, connAfter.SetupError, "successful reauth should leave no setup_error")
}

func TestIncrementalAuthorization_FailedReauthPreservesExistingCredentials(t *testing.T) {
	rig := newIncrementalAuthRig(t, "incremental-fail")
	browserCtx := rig.startBrowser(t)
	connID := rig.connectReadOnly(t, browserCtx)

	originalTokenID := rig.requireTokenScopes(t, connID, "read")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath))

	failure := helpers.ScriptAction{
		Status: 400,
		Body:   rfc6749Error("invalid_grant"),
	}
	rig.startReauthAndWaitForConsent(t, browserCtx, nil, &failure)
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath),
		"existing credential should remain usable while failed upgrade is still pending")

	rig.approveReauth(t, browserCtx)

	assert.Equal(t, originalTokenID, rig.requireTokenScopes(t, connID, "read"),
		"failed incremental auth must not replace the existing token row")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath),
		"failed upgrade must preserve proxy access with the existing token")

	connAfter := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, connAfter.State,
		"failed reauth should not demote an already configured connection")
	require.NotNil(t, connAfter.SetupStep)
	assert.True(t, connAfter.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"failed reauth should leave retryable auth_failed setup step; got %q", connAfter.SetupStep.String())
	require.NotNil(t, connAfter.SetupError)
	assert.Contains(t, *connAfter.SetupError, "received status code 400")
}
