//go:build integration

package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

const annotationMissingOptionalScopes = "oauth2.missing_optional_scopes"

// scopeMismatchSetup wires up a fresh provider client/user, env, and connector
// for one of the scope-mismatch scenarios. Tests script the token endpoint
// before invoking driveApprovalAndGetConnectionId so the token exchange that
// happens server-side after the user clicks Allow returns the desired scope
// shape.
type scopeMismatchSetup struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	clientKey    string
	userEmail    string
	userPassword string
	connector    sconfig.Connector
}

// newScopeMismatchSetup builds the test rig. Required scopes are appended to
// the connector as required; optional scopes are appended with Required=false.
// The provider client is registered with the union (required + optional + any
// extras the test plans to grant) so the authorize endpoint accepts the
// request — divergence is then introduced via the token-endpoint script.
func newScopeMismatchSetup(t *testing.T, name string, required, optional []string, registeredAtProvider []string) *scopeMismatchSetup {
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
		Scopes:         required,
		OptionalScopes: optional,
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
		Scope:                   strings.Join(registeredAtProvider, " "),
	})
	require.Equal(t, clientKey, registered.Key)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &scopeMismatchSetup{
		provider:     provider,
		env:          env,
		clientKey:    clientKey,
		userEmail:    userEmail,
		userPassword: userPassword,
		connector:    connector,
	}
}

// driveApprovalAndGetConnectionId walks chromedp from the marketplace's
// /connectors page through provider login + Allow, waits for the SPA's
// /connections page to render (proof the callback completed regardless of
// success or auth_failed), and returns the new connection's ID.
func (s *scopeMismatchSetup) driveApprovalAndGetConnectionId(t *testing.T) string {
	t.Helper()

	authToken, err := s.env.PublicAuthUtil.GenerateBearerToken(
		context.Background(),
		"test-actor",
		sconfig.RootNamespace,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)

	connectorsURL := s.env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Connect']`, chromedp.BySearch),
		chromedp.WaitVisible(`input[name="email"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, s.userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, s.userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="allow"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))

	page := s.env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Lenf(t, page.Results, 1, "expected exactly one connection after Connect; got %d", len(page.Results))
	return page.Results[0].Id.String()
}

// fetchConnectionScopes hits GET /api/v1/connections/{id}/scopes through the
// real HTTP server using a signed bearer for the test actor. Returns the JSON
// body and the response status.
func (s *scopeMismatchSetup) fetchConnectionScopes(t *testing.T, connectionID string) (int, map[string][]string) {
	t.Helper()

	path := "/api/v1/connections/" + connectionID + "/scopes"
	req, err := s.env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		path,
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	abs, err := url.Parse(s.env.ServerURL + path)
	require.NoError(t, err)
	req.URL = abs
	req.Host = abs.Host
	req.RequestURI = ""

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}

	var out map[string][]string
	require.NoErrorf(t, json.Unmarshal(body, &out), "decode scopes body: %s", string(body))
	return resp.StatusCode, out
}

func ptr(s string) *string { return &s }

// TestScopeMismatch_RequiredMissing — the connector requires {read, write} but
// the provider grants only {read}. The proxy must reject the token, record an
// auth failure, and surface the missing required scope in setup_error. The
// token row is still persisted so /scopes can show what was requested vs
// granted (useful for debugging the failure).
//
// See scope_mismatch_test.md for the scenario specification.
func TestScopeMismatch_RequiredMissing(t *testing.T) {
	s := newScopeMismatchSetup(t, "scope-required-missing",
		[]string{"read", "write"}, nil,
		[]string{"read", "write"})

	s.provider.Script(s.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read"),
	})

	connectionID := s.driveApprovalAndGetConnectionId(t)

	conn := s.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateCreated, conn.State,
		"connection should remain in created state when required scopes are missing")
	require.NotNilf(t, conn.SetupStep, "auth-failed connection should have a setup_step")
	assert.Truef(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"connection should be in auth_failed setup step; got %q", conn.SetupStep.String())
	require.NotNilf(t, conn.SetupError, "auth-failed connection should have setup_error recorded")
	assert.Containsf(t, *conn.SetupError, "required oauth2 scopes were not granted",
		"setup_error should call out the missing required scopes; got %q", *conn.SetupError)
	assert.Containsf(t, *conn.SetupError, "write",
		"setup_error should name the missing scope; got %q", *conn.SetupError)

	token := s.env.GetOAuth2Token(t, connectionID)
	require.NotNilf(t, token, "token row should be persisted even on required-missing so /scopes can show the divergence")
	assert.Equal(t, "read", token.Scopes, "stored scopes should reflect what the provider returned")
	assert.Equal(t, "read write", token.RequestedScopes,
		"requested_scopes should preserve the original request from the connector")

	annotations := s.env.GetConnection(t, connectionID).Annotations
	_, hasAnnotation := annotations[annotationMissingOptionalScopes]
	assert.False(t, hasAnnotation, "missing-required path should not write the optional-scope annotation")
}

// TestScopeMismatch_OptionalMissing — connector declares {read (req), write
// (opt)}, provider grants only {read}. Connection should land ready, with
// the missing-optional-scopes annotation set to "write" so callers can decide
// which features to expose.
func TestScopeMismatch_OptionalMissing(t *testing.T) {
	s := newScopeMismatchSetup(t, "scope-optional-missing",
		[]string{"read"}, []string{"write"},
		[]string{"read", "write"})

	s.provider.Script(s.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read"),
	})

	connectionID := s.driveApprovalAndGetConnectionId(t)

	conn := s.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State,
		"missing-optional should not block the connection from going ready")

	annotations := conn.Annotations
	require.Containsf(t, annotations, annotationMissingOptionalScopes,
		"missing-optional should be recorded on the connection as an annotation; got %v", annotations)
	assert.Equal(t, "write", annotations[annotationMissingOptionalScopes],
		"annotation value should list the missing optional scope")

	token := s.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token, "token must be persisted on the success path")
	assert.Equal(t, "read", token.Scopes)
	assert.Equal(t, "read write", token.RequestedScopes)

	status, body := s.fetchConnectionScopes(t, connectionID)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []string{"read", "write"}, body["requested"])
	assert.ElementsMatch(t, []string{"read"}, body["granted"])
}

// TestScopeMismatch_AllScopesGranted — connector declares {read (req), write
// (opt)} and the provider grants both. Connection ready, no annotation, no
// extras logged. Confirms the no-mismatch path doesn't write the
// missing-optional annotation.
func TestScopeMismatch_AllScopesGranted(t *testing.T) {
	s := newScopeMismatchSetup(t, "scope-all-granted",
		[]string{"read"}, []string{"write"},
		[]string{"read", "write"})

	s.provider.Script(s.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read write"),
	})

	connectionID := s.driveApprovalAndGetConnectionId(t)

	conn := s.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State)
	_, hasAnnotation := conn.Annotations[annotationMissingOptionalScopes]
	assert.False(t, hasAnnotation, "no annotation should be set when all declared scopes are granted")

	token := s.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token)
	assert.Equal(t, "read write", token.Scopes)
	assert.Equal(t, "read write", token.RequestedScopes)

	status, body := s.fetchConnectionScopes(t, connectionID)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []string{"read", "write"}, body["requested"])
	assert.ElementsMatch(t, []string{"read", "write"}, body["granted"])
}

// TestScopeMismatch_ExtraGranted — connector declares {read} but the provider
// grants {read, admin}. Extras don't block the connection; the proxy logs
// them but stores what came back so callers can see the actual capability set
// via /scopes.
func TestScopeMismatch_ExtraGranted(t *testing.T) {
	s := newScopeMismatchSetup(t, "scope-extra-granted",
		[]string{"read"}, nil,
		[]string{"read", "admin"})

	s.provider.Script(s.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr("read admin"),
	})

	connectionID := s.driveApprovalAndGetConnectionId(t)

	conn := s.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State,
		"extra scopes should not block the connection from going ready")
	_, hasAnnotation := conn.Annotations[annotationMissingOptionalScopes]
	assert.False(t, hasAnnotation, "extra-granted should not set the missing-optional annotation")

	token := s.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token)
	assert.Equal(t, "read admin", token.Scopes,
		"stored scopes should include the extras the provider granted")
	assert.Equal(t, "read", token.RequestedScopes)

	status, body := s.fetchConnectionScopes(t, connectionID)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []string{"read"}, body["requested"])
	assert.ElementsMatch(t, []string{"read", "admin"}, body["granted"])
}

// TestScopeMismatch_ProviderOmitsScope — RFC 6749 §5.1: when the token
// response omits the `scope` parameter, the granted set is identical to the
// request. The proxy must persist scopes=requested and treat the connection
// as fully satisfied (no missing/extra detection should fire).
func TestScopeMismatch_ProviderOmitsScope(t *testing.T) {
	s := newScopeMismatchSetup(t, "scope-omitted",
		[]string{"read"}, nil,
		[]string{"read"})

	// ScopeOverride to "" makes the token endpoint emit scope="" — which the
	// proxy treats the same as a missing field per RFC 6749 §5.1 (silent
	// agreement).
	s.provider.Script(s.clientKey, helpers.EndpointToken, helpers.ScriptAction{
		ScopeOverride: ptr(""),
	})

	connectionID := s.driveApprovalAndGetConnectionId(t)

	conn := s.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State)
	_, hasAnnotation := conn.Annotations[annotationMissingOptionalScopes]
	assert.False(t, hasAnnotation)

	token := s.env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token)
	assert.Equal(t, "read", token.Scopes,
		"omitted scope should fall back to the requested set")
	assert.Equal(t, "read", token.RequestedScopes)

	status, body := s.fetchConnectionScopes(t, connectionID)
	require.Equal(t, http.StatusOK, status)
	assert.ElementsMatch(t, []string{"read"}, body["requested"])
	assert.ElementsMatch(t, []string{"read"}, body["granted"])
}

// TestConnectionScopesEndpoint_NonOAuth2 — the endpoint must return 422 for
// any connection whose connector isn't OAuth2. We use a NoAuth connector
// created programmatically since chromedp isn't needed for this contract
// check.
func TestConnectionScopesEndpoint_NonOAuth2(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewNoAuthConnector(connectorID, "scope-endpoint-noauth", nil)

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:         helpers.ServiceTypeAPI,
		StartHTTPServer: true,
		Connectors:      []sconfig.Connector{connector},
	})
	defer env.Cleanup()

	connectionID := env.CreateConnection(t, connectorID, 1)

	path := "/api/v1/connections/" + connectionID + "/scopes"
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		path,
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	abs, err := url.Parse(env.ServerURL + path)
	require.NoError(t, err)
	req.URL = abs
	req.Host = abs.Host
	req.RequestURI = ""

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"GET /scopes on a non-OAuth2 connection must return 422")
}
