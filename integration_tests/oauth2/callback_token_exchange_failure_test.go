//go:build integration

package oauth2

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenExchangeFailureMessage mirrors the message string the production OAuth2
// package emits for every token-exchange failure (see
// internal/auth_methods/oauth2/token_exchange_failure.go:88). Operators key
// alerts off this exact string, so the tests pin it explicitly to catch
// silent renames as part of the public observability contract.
const tokenExchangeFailureMessage = "oauth token exchange failed"

// tokenExchangeFailureRig is the per-test fixture for the token-exchange
// rejection cases. Each case differs only in what the provider's /token
// endpoint returns — every other lever (connector shape, client
// registration, initiate flow, callback delivery) is identical.
type tokenExchangeFailureRig struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	logCapture   *helpers.LogCapture
	clientKey    string
	userID       string
	connectorID  apid.ID
	scopes       []string
	returnToURL  string
	errorPageURL string
}

// newTokenExchangeFailureRig wires a fresh provider client + user, env with
// LogCapture enabled, and a connector pointing at the provider. No HTTP
// server is started — every case delivers `/oauth2/callback` in-process via
// env.PublicGin, mirroring the rest of the direct-HTTP rejection suite.
func newTokenExchangeFailureRig(t *testing.T, name string) *tokenExchangeFailureRig {
	t.Helper()
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
	})

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
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
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &tokenExchangeFailureRig{
		provider:     provider,
		env:          env,
		logCapture:   logCapture,
		clientKey:    clientKey,
		userID:       user.ID,
		connectorID:  connectorID,
		scopes:       []string{"read"},
		returnToURL:  "https://example.com/return",
		errorPageURL: env.Cfg.GetRoot().ErrorPages.InternalError,
	}
}

// initiateAndMintCode kicks off a connection, drives the test provider's
// /test/authorize endpoint to mint a real authorization code, and returns
// (connectionID, stateID, code). Callers script the token endpoint between
// minting the code and delivering the callback.
func (r *tokenExchangeFailureRig) initiateAndMintCode(t *testing.T) (connectionID, stateID, code string) {
	t.Helper()
	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID = parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "InitiateOAuth2Connection should embed state_id: %s", redirectURL)

	authResp := r.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(r.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)
	pc, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	code = pc.Query().Get("code")
	require.NotEmptyf(t, code, "provider should issue a code on approve; got %s", authResp.RedirectURL)
	return connID, stateID, code
}

// requireOneFailureEvent asserts exactly one token-exchange failure event
// was emitted with the given category, and returns the parsed event so
// callers can pin additional fields.
func (r *tokenExchangeFailureRig) requireOneFailureEvent(t *testing.T, category string) map[string]any {
	t.Helper()
	events := r.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
	require.Lenf(t, events, 1, "expected exactly one token-exchange failure event; got %d (%v)", len(events), events)
	assert.Equal(t, category, events[0]["category"], "failure category mismatch")
	return events[0]
}

// requireAuthFailedConnection asserts the connection landed in the
// auth_failed terminal state via HandleAuthFailed: state stays `created`,
// setup_step=auth_failed, setup_error populated. The errorSubstring lets
// the caller pin a category-specific marker (e.g., the HTTP status code or
// a provider error string) without coupling to the full error wording.
func (r *tokenExchangeFailureRig) requireAuthFailedConnection(t *testing.T, connectionID, errorSubstring string) {
	t.Helper()
	conn := r.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateCreated, conn.State,
		"connection state should remain `created` on token-exchange failure")
	require.NotNilf(t, conn.SetupStep, "auth-failed connection should have a setup_step recorded")
	assert.Truef(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"connection should be in auth_failed setup step; got %q", conn.SetupStep.String())
	require.NotNilf(t, conn.SetupError, "auth-failed connection should have setup_error recorded")
	if errorSubstring != "" {
		assert.Containsf(t, *conn.SetupError, errorSubstring,
			"setup_error should mention %q; got %q", errorSubstring, *conn.SetupError)
	}
}

// requireRedirectToReturnURL asserts the proxy 302'd back to the
// connection's return_to_url with the setup-pending annotation. This is
// the post-failure redirect the marketplace UI uses to re-render the
// connection in its auth_failed state.
func (r *tokenExchangeFailureRig) requireRedirectToReturnURL(t *testing.T, connectionID, location string) {
	t.Helper()
	require.Truef(t, strings.HasPrefix(location, r.returnToURL),
		"failure should redirect to return_to_url with setup=pending; got %q", location)
	parsed, err := url.Parse(location)
	require.NoError(t, err)
	assert.Equal(t, "pending", parsed.Query().Get("setup"),
		"failure redirect should carry setup=pending so the UI re-renders the connection")
	assert.Equal(t, connectionID, parsed.Query().Get("connection_id"),
		"failure redirect should carry connection_id so the UI knows which connection failed")
}

// requireOneTokenCallObserved asserts exactly one /token call hit the
// provider — proves the proxy actually attempted the exchange and the
// failure was not short-circuited upstream of the token endpoint.
func (r *tokenExchangeFailureRig) requireOneTokenCallObserved(t *testing.T) {
	t.Helper()
	tokenReqs := r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: r.clientKey,
	})
	assert.Lenf(t, tokenReqs, 1, "expected exactly one /token call to the provider; got %d", len(tokenReqs))
}

// scriptTokenEndpoint enqueues a single response on the provider's /token
// endpoint scoped to this rig's client. Tests call this between minting
// the code and delivering the callback so the response is in place when
// the proxy posts to /token.
func (r *tokenExchangeFailureRig) scriptTokenEndpoint(action helpers.ScriptAction) {
	r.provider.Script(r.clientKey, helpers.EndpointToken, action)
}

// rfc6749Error is the standard token-endpoint error body shape per RFC
// 6749 §5.2. Used to deterministically reproduce the provider responses
// the proxy must classify into per-error categories.
func rfc6749Error(code string) string {
	return fmt.Sprintf(`{"error":%q}`, code)
}

// TestTokenExchangeRejection_InvalidGrant — scripted: provider returns
// `{"error":"invalid_grant"}` 400. This is the catch-all category for
// expired authorization codes, codes already used, and redirect_uri
// mismatches — providers fold all three into invalid_grant per RFC 6749
// §5.2. We rely on the spec's umbrella here so a single test covers
// issue #168 cases 1, 2, 3, 6, and 7.
func TestTokenExchangeRejection_InvalidGrant(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-invalid-grant")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("invalid_grant")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "invalid_grant")
	assert.Equal(t, stateID, event["state_id"])
	assert.Equal(t, float64(400), event["provider_status_code"])
	assert.Equal(t, "invalid_grant", event["provider_error"])

	require.Nil(t, rig.env.GetOAuth2Token(t, connID), "no token row should be persisted on token-exchange failure")
	rig.requireAuthFailedConnection(t, connID, "received status code 400")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_InvalidClient — scripted: provider returns
// `{"error":"invalid_client"}` 401. Covers issue #168 cases 4, 5, and 8
// (misconfigured client_id, misconfigured client_secret, generic
// invalid_client). Per RFC 6749 §5.2 a 401 with WWW-Authenticate is the
// canonical client-auth failure shape; we don't pin the WWW-Authenticate
// header because the proxy classifies on the body, not the header.
func TestTokenExchangeRejection_InvalidClient(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-invalid-client")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 401, Body: rfc6749Error("invalid_client")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "invalid_client")
	assert.Equal(t, stateID, event["state_id"])
	assert.Equal(t, float64(401), event["provider_status_code"])
	assert.Equal(t, "invalid_client", event["provider_error"])

	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "received status code 401")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_InvalidRequest — scripted: provider returns
// `{"error":"invalid_request"}` 400. RFC 6749 §5.2 reserves this for a
// malformed token-endpoint request; in production it would point at a
// proxy bug rather than user/provider state. We assert it gets its own
// category so SOC dashboards can split proxy-side regressions out from
// provider-side rejections.
func TestTokenExchangeRejection_InvalidRequest(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-invalid-request")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("invalid_request")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "invalid_request")
	assert.Equal(t, "invalid_request", event["provider_error"])
	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_UnauthorizedClient — scripted: provider
// returns `{"error":"unauthorized_client"}` 400. Means the client is
// recognized but not authorized for the authorization-code grant.
func TestTokenExchangeRejection_UnauthorizedClient(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-unauthorized-client")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("unauthorized_client")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "unauthorized_client")
	assert.Equal(t, "unauthorized_client", event["provider_error"])
	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_UnsupportedGrantType — scripted: provider
// returns `{"error":"unsupported_grant_type"}` 400. Indicates the
// provider does not support `grant_type=authorization_code` for this
// client — a configuration mismatch that the proxy surfaces with a
// distinct category so operators can spot connector misconfigurations.
func TestTokenExchangeRejection_UnsupportedGrantType(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-unsupported-grant")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("unsupported_grant_type")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "unsupported_grant_type")
	assert.Equal(t, "unsupported_grant_type", event["provider_error"])
	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_InvalidScopeError — scripted: provider
// returns `{"error":"invalid_scope"}` 400 at the token endpoint. Distinct
// from the scope-mismatch tests in scope_mismatch_test.go, which exercise
// the success-shape path where the token endpoint returns 200 with a
// narrower `scope` field — the proxy classifies that as a successful
// exchange + scope mismatch. Here the provider is rejecting the request
// outright at status-level, so the proxy must emit the invalid_scope
// failure category, not navigate the scope-mismatch logic.
func TestTokenExchangeRejection_InvalidScopeError(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-invalid-scope")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("invalid_scope")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "invalid_scope")
	assert.Equal(t, "invalid_scope", event["provider_error"])
	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_Provider4xxOther — scripted: 403 with a body
// that does not include any RFC 6749 §5.2 error code. The proxy should
// classify it as `provider_4xx_other` so the failure remains observable
// even when the provider deviates from the spec (e.g., a WAF page or a
// rate-limit error rendered as HTML). provider_error stays empty because
// we have nothing to safely log.
func TestTokenExchangeRejection_Provider4xxOther(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-4xx-other")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{
		Status:  403,
		Headers: map[string]string{"Content-Type": "text/html"},
		Body:    "<html><body>Forbidden by WAF</body></html>",
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "provider_4xx_other")
	assert.Equal(t, float64(403), event["provider_status_code"])
	_, hasProviderError := event["provider_error"]
	assert.Falsef(t, hasProviderError, "provider_error should be omitted when there is no recognized error code; got %v", event["provider_error"])

	require.Nil(t, rig.env.GetOAuth2Token(t, connID))
	rig.requireAuthFailedConnection(t, connID, "received status code 403")
	rig.requireOneTokenCallObserved(t)
}

// TestTokenExchangeRejection_MalformedResponse — scripted: provider
// returns 200 with a body that is not parseable as a token response.
// Distinct from 4xx failures — the request "succeeded" at the HTTP layer
// but the proxy could not extract an access_token, which is a data-shape
// failure on the provider side. Maps to the malformed_response category.
func TestTokenExchangeRejection_MalformedResponse(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "te-malformed")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{
		Status:       200,
		BodyTemplate: helpers.BodyMalformedJSON,
	})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)

	rig.requireOneFailureEvent(t, "malformed_response")
	require.Nil(t, rig.env.GetOAuth2Token(t, connID),
		"no token row should be persisted when the token response is unparseable")
	rig.requireAuthFailedConnection(t, connID, "")
	rig.requireOneTokenCallObserved(t)
}
