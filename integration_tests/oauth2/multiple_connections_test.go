//go:build integration

package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuth2MultipleConnections_SameTenantRefreshIsolation(t *testing.T) {
	rig := newProxyRefreshRig(t, "multiple-connections-same-tenant")
	secondUserEmail := "multiple-connections-second-user-" + apid.New(apid.PrefixActor).String() + "@example.com"
	secondUser := rig.provider.CreateUser(helpers.CreateUserRequest{
		Username:    secondUserEmail,
		Password:    "p4ssw0rd-second-user",
		Email:       secondUserEmail,
		DisplayName: "Second OAuth User",
	})

	connA := completeAuthFlowAsProviderUser(t, rig, rig.userID)
	connB := completeAuthFlowAsProviderUser(t, rig, secondUser.ID)
	require.NotEqual(t, connA, connB, "each initiate call must create a distinct connection")

	tokenA := rig.env.GetOAuth2Token(t, connA)
	tokenB := rig.env.GetOAuth2Token(t, connB)
	require.NotNil(t, tokenA)
	require.NotNil(t, tokenB)
	require.NotEqual(t, tokenA.Id, tokenB.Id, "connections must not share the same token row")

	refreshA := rig.env.DecryptOAuth2RefreshToken(t, tokenA)
	refreshB := rig.env.DecryptOAuth2RefreshToken(t, tokenB)
	require.NotEqual(t, refreshA, refreshB, "provider should issue independent refresh tokens per connection")

	rig.forceTokenExpired(t, connA, false)
	wA := rig.env.DoProxyRequest(t, connA, rig.provider.ResourceURL("/echo"), http.MethodGet)
	require.Equalf(t, http.StatusOK, wA.Code,
		"proxy through first connection should refresh and succeed; got %d body=%s", wA.Code, wA.Body.String())

	refreshedA := rig.env.GetOAuth2Token(t, connA)
	unchangedB := rig.env.GetOAuth2Token(t, connB)
	require.NotEqual(t, tokenA.Id, refreshedA.Id, "first connection should get a replacement token row")
	require.Equal(t, tokenB.Id, unchangedB.Id, "refreshing first connection must not replace second connection's token row")
	require.Equal(t, refreshB, rig.env.DecryptOAuth2RefreshToken(t, unchangedB),
		"refreshing first connection must not mutate second connection's refresh token")
	require.Len(t, refreshGrantRequests(rig), 1, "only the expired first connection should have refreshed")

	rig.forceTokenExpired(t, connB, false)
	wB := rig.env.DoProxyRequest(t, connB, rig.provider.ResourceURL("/echo"), http.MethodGet)
	require.Equalf(t, http.StatusOK, wB.Code,
		"proxy through second connection should refresh and succeed; got %d body=%s", wB.Code, wB.Body.String())

	refreshedB := rig.env.GetOAuth2Token(t, connB)
	require.NotEqual(t, tokenB.Id, refreshedB.Id, "second connection should get its own replacement token row")
	require.Len(t, refreshGrantRequests(rig), 2, "each expired connection should refresh exactly once")
}

func TestOAuth2MultipleConnections_DifferentTenantsSameProviderAccount(t *testing.T) {
	rig := newProxyRefreshRig(t, "multiple-connections-tenants")
	rig.provider.SetRefreshRotation(false)
	t.Cleanup(func() { rig.provider.SetRefreshRotation(true) })

	tenantA := uniqueTenantNamespace("tenant-a")
	tenantB := uniqueTenantNamespace("tenant-b")
	_, err := rig.env.Core.CreateNamespace(context.Background(), tenantA, nil)
	require.NoError(t, err)
	_, err = rig.env.Core.CreateNamespace(context.Background(), tenantB, nil)
	require.NoError(t, err)

	connA := completeAuthFlowAsActor(t, rig, "tenant-a-actor", tenantA)
	connB := completeAuthFlowAsActor(t, rig, "tenant-b-actor", tenantB)
	require.NotEqual(t, connA, connB)

	dbConnA := rig.env.GetConnection(t, connA)
	dbConnB := rig.env.GetConnection(t, connB)
	require.Equal(t, tenantA, dbConnA.Namespace)
	require.Equal(t, tenantB, dbConnB.Namespace)

	tokenA := rig.env.GetOAuth2Token(t, connA)
	tokenB := rig.env.GetOAuth2Token(t, connB)
	require.NotNil(t, tokenA)
	require.NotNil(t, tokenB)
	require.NotEqual(t, tokenA.Id, tokenB.Id, "tenant connections must store distinct token rows")

	rig.forceTokenExpired(t, connA, false)
	wA := doProxyRequestAsActor(t, rig.env, connA, rig.provider.ResourceURL("/echo"), http.MethodGet, "tenant-a-actor", tenantA)
	require.Equalf(t, http.StatusOK, wA.Code,
		"tenant A proxy should refresh and succeed; got %d body=%s", wA.Code, wA.Body.String())

	postA := rig.env.GetOAuth2Token(t, connA)
	postB := rig.env.GetOAuth2Token(t, connB)
	assert.NotEqual(t, tokenA.Id, postA.Id, "tenant A connection should refresh")
	assert.Equal(t, tokenB.Id, postB.Id, "tenant A refresh must not replace tenant B token row")

	rig.forceTokenExpired(t, connB, false)
	wB := doProxyRequestAsActor(t, rig.env, connB, rig.provider.ResourceURL("/echo"), http.MethodGet, "tenant-b-actor", tenantB)
	require.Equalf(t, http.StatusOK, wB.Code,
		"tenant B proxy should refresh and succeed; got %d body=%s", wB.Code, wB.Body.String())

	postB = rig.env.GetOAuth2Token(t, connB)
	assert.NotEqual(t, tokenB.Id, postB.Id, "tenant B connection should refresh independently")
	require.Len(t, refreshGrantRequests(rig), 2, "each tenant connection should refresh exactly once")
}

func completeAuthFlowAsProviderUser(t *testing.T, r *proxyRefreshRig, providerUserID string) string {
	t.Helper()
	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	return completeAuthFlowWithRedirect(t, r, connID, redirectURL, providerUserID)
}

func completeAuthFlowAsActor(t *testing.T, r *proxyRefreshRig, actorExternalID, namespace string) string {
	t.Helper()

	connID, redirectURL := r.env.InitiateOAuth2Connection(
		t,
		r.connectorID,
		r.returnToURL,
		helpers.WithActor(actorExternalID, namespace),
	)
	return completeAuthFlowWithRedirect(
		t,
		r,
		connID,
		redirectURL,
		r.userID,
		helpers.WithActor(actorExternalID, namespace),
	)
}

func completeAuthFlowWithRedirect(t *testing.T, r *proxyRefreshRig, connID, redirectURL, providerUserID string, callbackOpts ...helpers.OAuth2Option) string {
	t.Helper()

	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      providerUserID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(r.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	callback, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	loc := r.env.DeliverOAuth2Callback(
		t,
		r.env.ForgeOAuth2CallbackURL(stateID, code),
		callbackOpts...,
	)
	require.Truef(t, strings.HasPrefix(loc, r.returnToURL),
		"auth flow should land on return_to_url; got %q", loc)
	return connID
}

func doProxyRequestAsActor(t *testing.T, env *helpers.IntegrationTestEnv, connectionID, targetURL, method, actorExternalID, namespace string) *httptest.ResponseRecorder {
	t.Helper()
	require.NotNil(t, env.ApiGin, "doProxyRequestAsActor requires in-process API gin")

	proxyReq := coreIface.ProxyRequest{
		URL:    targetURL,
		Method: method,
	}
	body, err := json.Marshal(proxyReq)
	require.NoError(t, err)

	path := "/api/v1/connections/" + connectionID + "/_proxy"
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		path,
		bytes.NewReader(body),
		namespace,
		actorExternalID,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.ApiGin.ServeHTTP(w, req)
	return w
}

func uniqueTenantNamespace(label string) string {
	return fmt.Sprintf("%s.%s-%s", sconfig.RootNamespace, label, apid.New(apid.PrefixConnection).String())
}
