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

// Scenario 19 from issue #178: providers differ on how they require OAuth2
// clients to authenticate to /token. These tests drive the production callback
// handler against go-oauth2-server clients registered for each supported method
// and assert the provider accepts the configured shape.

type authMethodCompatibilityRig struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	logCapture   *helpers.LogCapture
	clientKey    string
	clientSecret string
	userID       string
	connectorID  apid.ID
	returnToURL  string
}

func newAuthMethodCompatibilityRig(
	t *testing.T,
	name string,
	connectorMethod cschema.TokenEndpointAuthMethod,
	providerMethod helpers.TokenEndpointAuthMethod,
	pkce bool,
) *authMethodCompatibilityRig {
	t.Helper()

	provider := helpers.NewOAuth2TestProvider(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := name + "-client-" + suffix
	clientSecret := name + "-secret-" + suffix
	userEmail := name + "-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	opts := helpers.OAuth2ConnectorOptions{
		ClientID:                clientKey,
		ClientSecret:            clientSecret,
		TokenEndpointAuthMethod: connectorMethod,
		Scopes:                  []string{"read"},
	}
	if pkce {
		opts.ClientSecret = ""
		opts.PKCE = &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256}
	}
	connector := helpers.NewOAuth2Connector(connectorID, name, provider, opts)

	logCapture := helpers.NewLogCapture()
	env := helpers.Setup(t, helpers.SetupOptions{
		Service:       helpers.ServiceTypeAPI,
		IncludePublic: true,
		Connectors:    []sconfig.Connector{connector},
		LogCapture:    logCapture,
	})
	t.Cleanup(env.Cleanup)

	createClient := helpers.CreateClientRequest{
		Key:                     clientKey,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: providerMethod,
		Scope:                   "read",
		RequirePKCE:             pkce,
	}
	if !pkce {
		createClient.Secret = clientSecret
	}
	registered := provider.CreateClient(createClient)
	require.Equal(t, clientKey, registered.Key)
	require.Equal(t, providerMethod, registered.TokenEndpointAuthMethod)
	require.Equal(t, pkce, registered.RequirePKCE)

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: "irrelevant-test-password",
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	return &authMethodCompatibilityRig{
		provider:     provider,
		env:          env,
		logCapture:   logCapture,
		clientKey:    clientKey,
		clientSecret: clientSecret,
		userID:       user.ID,
		connectorID:  connectorID,
		returnToURL:  env.Cfg.GetRoot().Public.GetBaseUrl() + "/connections",
	}
}

func (r *authMethodCompatibilityRig) completeAuthFlow(t *testing.T, pkce bool) (connectionID string, tokenReq helpers.RecordedRequest) {
	t.Helper()

	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	parsedRedirect, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsedRedirect.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authReq := helpers.AuthorizeRequest{
		ClientID:    r.clientKey,
		UserID:      r.userID,
		RedirectURI: r.env.PublicOAuthCallbackURL(),
		Scope:       "read",
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	}
	if pkce {
		upstreamAuthorizeURL := r.env.FollowOAuth2Redirect(t, redirectURL)
		upstreamParsed, err := url.Parse(upstreamAuthorizeURL)
		require.NoError(t, err)
		authReq.CodeChallenge = upstreamParsed.Query().Get("code_challenge")
		authReq.CodeChallengeMethod = upstreamParsed.Query().Get("code_challenge_method")
		require.NotEmptyf(t, authReq.CodeChallenge,
			"public-client flow must emit a PKCE code_challenge; authorize URL=%s", upstreamAuthorizeURL)
		require.NotEmpty(t, authReq.CodeChallengeMethod)
	}

	beforeCallback := time.Now().Add(-1 * time.Second)
	authResp := r.provider.Authorize(authReq)
	require.NotEmpty(t, authResp.RedirectURL)

	loc := r.env.DeliverOAuth2Callback(t, authResp.RedirectURL)
	require.Truef(t, strings.HasPrefix(loc, r.returnToURL),
		"successful auth method flow should redirect to return_to_url; got %q", loc)

	token := r.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token, "successful auth method flow must persist a token")
	conn := r.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateConfigured, conn.State)

	tokenReqs := r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: r.clientKey,
		Since:    beforeCallback,
	})
	require.Lenf(t, tokenReqs, 1, "expected exactly one /token request for %s; got %d", r.clientKey, len(tokenReqs))
	return connID, tokenReqs[0]
}

func recordedHeader(headers map[string]string, name string) string {
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func TestProviderAuthMethodCompatibility_ClientSecretBasic(t *testing.T) {
	rig := newAuthMethodCompatibilityRig(t,
		"auth-method-basic",
		cschema.TokenEndpointAuthClientSecretBasic,
		helpers.TokenEndpointAuthBasic,
		false,
	)

	_, tokenReq := rig.completeAuthFlow(t, false)
	authHeader := recordedHeader(tokenReq.Headers, "Authorization")
	assert.Truef(t, strings.HasPrefix(authHeader, "Basic "),
		"client_secret_basic must authenticate with Basic auth; got %q", authHeader)
	assert.Empty(t, lastForm(tokenReq.Form, "client_id"),
		"client_secret_basic must not duplicate client_id in the form body")
	assert.Empty(t, lastForm(tokenReq.Form, "client_secret"),
		"client_secret_basic must not duplicate client_secret in the form body")
	assert.NotContains(t, authHeader, rig.clientSecret,
		"client secret must not be exposed in plaintext in the recorded Authorization header")
}

func TestProviderAuthMethodCompatibility_ClientSecretPost(t *testing.T) {
	rig := newAuthMethodCompatibilityRig(t,
		"auth-method-post",
		cschema.TokenEndpointAuthClientSecretPost,
		helpers.TokenEndpointAuthPost,
		false,
	)

	_, tokenReq := rig.completeAuthFlow(t, false)
	assert.Empty(t, recordedHeader(tokenReq.Headers, "Authorization"),
		"client_secret_post must not use Authorization header")
	assert.Equal(t, rig.clientKey, lastForm(tokenReq.Form, "client_id"))
	assert.NotEmpty(t, lastForm(tokenReq.Form, "client_secret"),
		"client_secret_post must send client_secret in the form body")
	assert.NotEqual(t, rig.clientSecret, lastForm(tokenReq.Form, "client_secret"),
		"provider recorder should redact client_secret even though it was accepted")
}

func TestProviderAuthMethodCompatibility_PublicClientPKCE(t *testing.T) {
	rig := newAuthMethodCompatibilityRig(t,
		"auth-method-none",
		cschema.TokenEndpointAuthNone,
		helpers.TokenEndpointAuthNone,
		true,
	)

	_, tokenReq := rig.completeAuthFlow(t, true)
	assert.Empty(t, recordedHeader(tokenReq.Headers, "Authorization"),
		"public client must not use Authorization header")
	assert.Equal(t, rig.clientKey, lastForm(tokenReq.Form, "client_id"))
	assert.Empty(t, lastForm(tokenReq.Form, "client_secret"),
		"public client must not send client_secret")
	assert.True(t, hasCodeVerifierForm(tokenReq.Form),
		"public-client flow must send PKCE code_verifier at /token")
}

func TestProviderAuthMethodCompatibility_InvalidMethodFailsClearly(t *testing.T) {
	rig := newAuthMethodCompatibilityRig(t,
		"auth-method-invalid",
		cschema.TokenEndpointAuthClientSecretPost,
		helpers.TokenEndpointAuthBasic,
		false,
	)

	connID, redirectURL := rig.env.InitiateOAuth2Connection(t, rig.connectorID, rig.returnToURL)
	parsedRedirect, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsedRedirect.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := rig.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    rig.clientKey,
		UserID:      rig.userID,
		RedirectURI: rig.env.PublicOAuthCallbackURL(),
		Scope:       "read",
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)

	loc := rig.env.DeliverOAuth2Callback(t, authResp.RedirectURL)
	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"invalid auth method should redirect to return_to_url with setup=pending; got %q", loc)
	parsedLoc, err := url.Parse(loc)
	require.NoError(t, err)
	assert.Equal(t, "pending", parsedLoc.Query().Get("setup"))
	assert.Equal(t, connID, parsedLoc.Query().Get("connection_id"))

	require.Nil(t, rig.env.GetOAuth2Token(t, connID),
		"token row must not persist when provider rejects the configured client auth method")
	conn := rig.env.GetConnection(t, connID)
	assert.Equal(t, database.ConnectionStateSetup, conn.State)
	require.NotNil(t, conn.SetupStep)
	assert.True(t, conn.SetupStep.Equals(cschema.SetupStepAuthFailed),
		"connection should land in auth_failed after invalid token-endpoint auth method")
	require.NotNil(t, conn.SetupError)
	assert.NotContains(t, *conn.SetupError, rig.clientSecret,
		"setup_error must not leak client_secret")

	events := rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
	require.Lenf(t, events, 1, "expected exactly one token-exchange failure event; got %d (%v)", len(events), events)
	category, _ := events[0]["category"].(string)
	assert.Containsf(t, []string{"invalid_client", "provider_4xx_other"}, category,
		"invalid client-auth method should be classified as invalid_client or provider_4xx_other; got %q", category)

	assertConnectionIDPresent(t, rig.logCapture, connID)
	assertNoSecretsInLogs(t, rig.logCapture, flowSecrets{
		ClientSecret: rig.clientSecret,
	})
}
