//go:build integration

package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// Scenario 26 from issue #181: proxy-initiated disconnect revokes provider
// credentials, removes local usability, and blocks future proxy calls.

type disconnectRevocationRig struct {
	provider    *helpers.OAuth2TestProvider
	env         *helpers.IntegrationTestEnv
	clientKey   string
	userID      string
	connectorID apid.ID
	returnToURL string
}

func newDisconnectRevocationRig(t *testing.T, name string) *disconnectRevocationRig {
	t.Helper()

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := time.Now().Format("20060102150405.000000000")
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, helpers.OAuth2ConnectorOptions{
		ClientID:          clientKey,
		ClientSecret:      clientSecret,
		Scopes:            []string{"read"},
		IncludeRevocation: true,
	})
	oauthAuth := connector.Auth.InnerVal.(*cschema.AuthOAuth2)
	supportedTokens := cschema.AuthOAuth2RevocationSupportedTypeRefreshToken
	oauthAuth.Revocation.SupportedTokens = &supportedTokens
	oauthAuth.Revocation.FormOverrides = map[string]string{
		"client_id":     clientKey,
		"client_secret": clientSecret,
	}

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
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

	return &disconnectRevocationRig{
		provider:    provider,
		env:         env,
		clientKey:   clientKey,
		userID:      user.ID,
		connectorID: connectorID,
		returnToURL: env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}
}

func (r *disconnectRevocationRig) completeAuthFlow(t *testing.T) string {
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
		Scope:       "read",
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	callback, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	loc := r.env.DeliverOAuth2Callback(t, r.env.ForgeOAuth2CallbackURL(stateID, code))
	require.Contains(t, loc, r.returnToURL)
	return connID
}

func (r *disconnectRevocationRig) disconnect(t *testing.T, connectionID string) {
	t.Helper()

	reqBody, err := json.Marshal(schemaapi.DisconnectConnectionRequestJson{
		TimeoutSeconds: int64Ptr(10),
	})
	require.NoError(t, err)

	path := "/api/v1/connections/" + connectionID + "/_disconnect"
	req, err := r.env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		path,
		bytes.NewReader(reqBody),
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.env.ApiGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusOK, w.Code, "disconnect failed: %s", w.Body.String())

	var body struct {
		Connection struct {
			State string `json:"state"`
		} `json:"connection"`
		TaskID string `json:"task_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, string(database.ConnectionStateDisconnecting), body.Connection.State)
	assert.NotEmpty(t, body.TaskID)

	helpers.RequireWorkflowTaskCompleted(t, r.env, body.TaskID, 15*time.Second)
}

func int64Ptr(v int64) *int64 {
	return &v
}

func requireConnectionDeleted(t *testing.T, rig *disconnectRevocationRig, connectionID string) {
	t.Helper()

	id, err := apid.Parse(connectionID)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := rig.env.Db.GetConnection(context.Background(), id)
		return errors.Is(err, database.ErrNotFound)
	}, 10*time.Second, 100*time.Millisecond, "connection should be soft-deleted after disconnect workflow")
}

func requireProxyBlockedAfterDisconnect(t *testing.T, rig *disconnectRevocationRig, connectionID string) {
	t.Helper()

	requireProxyBlockedForProvider(t, rig.env, rig.provider, connectionID)
}

func requireProxyBlockedForProvider(
	t *testing.T,
	env *helpers.IntegrationTestEnv,
	provider *helpers.OAuth2TestProvider,
	connectionID string,
) {
	t.Helper()

	w := env.DoProxyRequest(t, connectionID, provider.ResourceURL("/echo"), http.MethodGet)
	assert.Equalf(t, http.StatusNotFound, w.Code,
		"future proxied calls should require reconnect after disconnect; body=%s", w.Body.String())
}

func TestDisconnectRevocation_RevokesProviderTokensAndBlocksFutureProxy(t *testing.T) {
	rig := newDisconnectRevocationRig(t, "disconnect-revoke")
	connID := rig.completeAuthFlow(t)

	token := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token)
	accessToken := rig.env.DecryptOAuth2AccessToken(t, token)
	refreshToken := rig.env.DecryptOAuth2RefreshToken(t, token)

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), http.MethodGet)
	require.Equal(t, http.StatusOK, parseRevocationProxyResponse(t, w).StatusCode)

	helpers.StartCoreWorkflowWorker(t, rig.env)
	rig.disconnect(t, connID)
	requireConnectionDeleted(t, rig, connID)

	assert.Nil(t, rig.env.GetOAuth2Token(t, connID), "successful disconnect should tombstone the local OAuth token row")
	requireProxyBlockedAfterDisconnect(t, rig, connID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.clientKey,
	})
	require.Lenf(t, revokeReqs, 1, "expected one refresh-token revocation request; got %d", len(revokeReqs))
	assert.Equal(t, "refresh_token", lastForm(revokeReqs[0].Form, "token_type_hint"))
	assert.Equal(t, refreshToken, lastForm(revokeReqs[0].Form, "token"))
	assert.NotEqual(t, accessToken, lastForm(revokeReqs[0].Form, "token"))
	assert.Equal(t, rig.clientKey, lastForm(revokeReqs[0].Form, "client_id"))
}

func TestDisconnectRevocation_RevocationFailureStillCompletesDisconnect(t *testing.T) {
	rig := newDisconnectRevocationRig(t, "disconnect-revoke-fail")
	connID := rig.completeAuthFlow(t)

	require.NotNil(t, rig.env.GetOAuth2Token(t, connID))
	rig.provider.Script("", helpers.EndpointRevoke, helpers.ScriptAction{
		Status:    http.StatusServiceUnavailable,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	helpers.StartCoreWorkflowWorker(t, rig.env)
	rig.disconnect(t, connID)
	requireConnectionDeleted(t, rig, connID)
	requireProxyBlockedAfterDisconnect(t, rig, connID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.clientKey,
	})
	assert.Lenf(t, revokeReqs, 3,
		"disconnect should exhaust the revocation retry budget, then proceed with local disconnect")
}
