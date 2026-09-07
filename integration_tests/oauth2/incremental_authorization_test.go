//go:build integration

package oauth2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
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
	userID       string
	connectorID  apid.ID
	resourcePath string
	returnToURL  string
}

func newIncrementalAuthRig(t *testing.T, name string) *incrementalAuthRig {
	t.Helper()

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnector)
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
		userID:       user.ID,
		connectorID:  connectorID,
		resourcePath: "/echo",
		returnToURL:  env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}
}

func (r *incrementalAuthRig) connectReadOnly(t *testing.T) string {
	t.Helper()

	r.provider.Script(r.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read"),
	})

	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	r.approveOAuthRedirect(t, redirectURL, ptr("read"))

	return connID
}

func (r *incrementalAuthRig) startReauth(t *testing.T, connectionID string, scopeOverride *string, failure *helpers.ScriptAction) string {
	t.Helper()
	if scopeOverride != nil {
		r.provider.Script(r.clientKey, helpers.EndpointToken, helpers.ScriptAction{
			ScopeOverride: scopeOverride,
		})
	}
	if failure != nil {
		r.provider.Script(r.clientKey, helpers.EndpointToken, *failure)
	}

	return r.env.ReauthOAuth2Connection(t, connectionID, r.returnToURL)
}

func (r *incrementalAuthRig) approveOAuthRedirect(t *testing.T, redirectURL string, scopeOverride *string) {
	t.Helper()

	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("stateId")
	require.NotEmpty(t, stateID, "redirect should embed stateId: %s", redirectURL)

	scope := "read_write"
	if scopeOverride != nil {
		scope = *scopeOverride
	}
	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       scope,
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	callback, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	loc := r.env.DeliverOAuth2Callback(t, r.env.ForgeOAuth2CallbackURL(stateID, code))
	require.Truef(t, strings.HasPrefix(loc, r.returnToURL),
		"auth flow should land on returnToUrl; got %q", loc)
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

	var list schemaapi.ConnectionScopeList
	require.NoErrorf(t, json.Unmarshal(body, &list), "decode scopes body: %s", string(body))
	requested := make([]string, 0, len(list.Items))
	granted := make([]string, 0, len(list.Items))
	for _, scope := range list.Items {
		if scope.Requested {
			requested = append(requested, scope.Name)
		}
		if scope.Granted {
			granted = append(granted, scope.Name)
		}
	}
	assert.ElementsMatch(t, wantRequested, requested)
	assert.ElementsMatch(t, wantGranted, granted)
}

func TestIncrementalAuthorization_ReauthUpgradesScopes(t *testing.T) {
	rig := newIncrementalAuthRig(t, "incremental-success")
	connID := rig.connectReadOnly(t)

	originalTokenID := rig.requireTokenScopes(t, connID, "read")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath))

	reauthRedirectURL := rig.startReauth(t, connID, ptr("read_write"), nil)

	connDuring := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, connDuring.State)
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath),
		"existing credential should remain usable while upgrade consent is pending")
	assert.Equal(t, originalTokenID, rig.requireTokenScopes(t, connID, "read"),
		"pending upgrade must not replace the stored token")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})

	rig.approveOAuthRedirect(t, reauthRedirectURL, ptr("read_write"))

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
	connID := rig.connectReadOnly(t)

	originalTokenID := rig.requireTokenScopes(t, connID, "read")
	rig.requireConnectionScopes(t, connID, []string{"read_write"}, []string{"read"})
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath))

	failure := helpers.ScriptAction{
		Status: 400,
		Body:   rfc6749Error("invalid_grant"),
	}
	reauthRedirectURL := rig.startReauth(t, connID, nil, &failure)
	assert.Equal(t, http.StatusOK, rig.proxyResourceStatus(t, connID, rig.resourcePath),
		"existing credential should remain usable while failed upgrade is still pending")

	rig.approveOAuthRedirect(t, reauthRedirectURL, ptr("read_write"))

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
