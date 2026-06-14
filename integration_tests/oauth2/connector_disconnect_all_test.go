//go:build integration

package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type connectorDisconnectAllRig struct {
	provider    *helpers.OAuth2TestProvider
	env         *helpers.IntegrationTestEnv
	connectors  []connectorDisconnectAllConnector
	userID      string
	returnToURL string
}

type connectorDisconnectAllConnector struct {
	id        apid.ID
	clientKey string
}

func newConnectorDisconnectAllRig(t *testing.T, name string, connectorCount int) *connectorDisconnectAllRig {
	t.Helper()

	require.GreaterOrEqual(t, connectorCount, 1)

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := time.Now().Format("20060102150405.000000000")

	connectors := make([]sconfig.Connector, 0, connectorCount)
	rigConnectors := make([]connectorDisconnectAllConnector, 0, connectorCount)
	for i := 0; i < connectorCount; i++ {
		clientKey := fmt.Sprintf("%s-client-%d-%s", name, i, suffix)
		clientSecret := fmt.Sprintf("%s-secret-%d-%s", name, i, suffix)
		connectorID := apid.New(apid.PrefixConnectorVersion)
		connector := helpers.NewOAuth2Connector(connectorID, fmt.Sprintf("%s-%d", name, i), provider, helpers.OAuth2ConnectorOptions{
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

		connectors = append(connectors, connector)
		rigConnectors = append(rigConnectors, connectorDisconnectAllConnector{
			id:        connectorID,
			clientKey: clientKey,
		})
	}

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    connectors,
	})
	t.Cleanup(env.Cleanup)

	for i, connector := range rigConnectors {
		clientSecret := fmt.Sprintf("%s-secret-%d-%s", name, i, suffix)
		registered := provider.CreateClient(helpers.CreateClientRequest{
			Key:                     connector.clientKey,
			Secret:                  clientSecret,
			RedirectURI:             env.PublicOAuthCallbackURL(),
			TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
			Scope:                   "read",
		})
		require.Equal(t, connector.clientKey, registered.Key)
	}

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: name + "-" + suffix + "@example.com",
		Password: "p4ssw0rd-" + suffix,
		Email:    name + "-" + suffix + "@example.com",
	})
	require.NotEmpty(t, user.ID)

	return &connectorDisconnectAllRig{
		provider:    provider,
		env:         env,
		connectors:  rigConnectors,
		userID:      user.ID,
		returnToURL: env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}
}

func (r *connectorDisconnectAllRig) completeAuthFlow(t *testing.T, connector connectorDisconnectAllConnector) string {
	t.Helper()

	connID, redirectURL := r.env.InitiateOAuth2Connection(t, connector.id, r.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    connector.clientKey,
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

func (r *connectorDisconnectAllRig) disconnectAll(t *testing.T, connectorID apid.ID, timeoutSeconds int64) {
	t.Helper()

	reqBody, err := json.Marshal(schemaapi.ConnectorLifecycleRequestJson{
		TimeoutSeconds: int64Ptr(timeoutSeconds),
	})
	require.NoError(t, err)

	path := "/api/v1/connectors/" + connectorID.String() + "/_disconnect_all"
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
	require.Equalf(t, http.StatusOK, w.Code, "disconnect all failed: %s", w.Body.String())

	var body schemaapi.ConnectorLifecycleResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, connectorID, body.ConnectorId)
	require.NotEmpty(t, body.TaskId)

	requireWorkflowTaskCompleted(t, r.env, body.TaskId, "test-actor", time.Duration(timeoutSeconds+5)*time.Second)
}

func requireConnectionDeletedByID(t *testing.T, env *helpers.IntegrationTestEnv, connectionID string) {
	t.Helper()

	id, err := apid.Parse(connectionID)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := env.Db.GetConnection(context.Background(), id)
		return errors.Is(err, database.ErrNotFound)
	}, 10*time.Second, 100*time.Millisecond, "connection should be soft-deleted after connector disconnect-all")
}

func requireConnectionAvailable(t *testing.T, rig *connectorDisconnectAllRig, connectionID string) {
	t.Helper()

	id, err := apid.Parse(connectionID)
	require.NoError(t, err)

	conn, err := rig.env.Db.GetConnection(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, database.ConnectionStateConfigured, conn.State)
	require.NotNil(t, rig.env.GetOAuth2Token(t, connectionID))

	w := rig.env.DoProxyRequest(t, connectionID, rig.provider.ResourceURL("/echo"), http.MethodGet)
	require.Equal(t, http.StatusOK, parseRevocationProxyResponse(t, w).StatusCode)
}

func TestConnectorDisconnectAll_DisconnectsTargetConnectionsOnly(t *testing.T) {
	rig := newConnectorDisconnectAllRig(t, "connector-disconnect-all", 2)

	targetConnection1 := rig.completeAuthFlow(t, rig.connectors[0])
	targetConnection2 := rig.completeAuthFlow(t, rig.connectors[0])
	otherConnection := rig.completeAuthFlow(t, rig.connectors[1])

	requireConnectionAvailable(t, rig, targetConnection1)
	requireConnectionAvailable(t, rig, targetConnection2)
	requireConnectionAvailable(t, rig, otherConnection)

	startCoreWorkflowWorker(t, rig.env)
	rig.disconnectAll(t, rig.connectors[0].id, 20)

	requireConnectionDeletedByID(t, rig.env, targetConnection1)
	requireConnectionDeletedByID(t, rig.env, targetConnection2)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, targetConnection1)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, targetConnection2)
	requireConnectionAvailable(t, rig, otherConnection)

	targetRevokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[0].clientKey,
	})
	assert.Len(t, targetRevokeReqs, 2)

	otherRevokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[1].clientKey,
	})
	assert.Empty(t, otherRevokeReqs)
}

func TestConnectorDisconnectAll_RevocationFailureStillCompletes(t *testing.T) {
	rig := newConnectorDisconnectAllRig(t, "connector-disconnect-all-fail", 1)

	connectionID := rig.completeAuthFlow(t, rig.connectors[0])
	require.NotNil(t, rig.env.GetOAuth2Token(t, connectionID))
	rig.provider.Script(rig.connectors[0].clientKey, helpers.EndpointRevoke, helpers.ScriptAction{
		Status:    http.StatusServiceUnavailable,
		Body:      `{"error":"temporarily_unavailable"}`,
		FailCount: 10,
	})

	startCoreWorkflowWorker(t, rig.env)
	rig.disconnectAll(t, rig.connectors[0].id, 20)

	requireConnectionDeletedByID(t, rig.env, connectionID)
	requireProxyBlockedForProvider(t, rig.env, rig.provider, connectionID)

	revokeReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointRevoke,
		ClientID: rig.connectors[0].clientKey,
	})
	assert.Lenf(t, revokeReqs, 3,
		"disconnect-all should allow child disconnect to exhaust revocation retries, then force local disconnect")
}
