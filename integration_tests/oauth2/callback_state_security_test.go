//go:build integration

package oauth2

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectionEventMessage mirrors the message string the production OAuth2
// package emits for every rejected callback. Operators key alerts off
// this exact string, so the tests pin it explicitly to catch silent
// renames as part of the public observability contract.
const rejectionEventMessage = "oauth callback rejected"

// callbackStateSecurityRig is the per-test fixture for the four
// direct-HTTP rejection cases. All four share the same OAuth2 connector
// shape and provider client setup; only the malformed/tampered/replayed
// state value varies between them.
type callbackStateSecurityRig struct {
	provider     *helpers.OAuth2TestProvider
	env          *helpers.IntegrationTestEnv
	logCapture   *helpers.LogCapture
	clientKey    string
	userID       string
	connectorID  apid.ID
	connector    sconfig.Connector
	scopes       []string
	returnToURL  string
	errorPageURL string
}

// newCallbackStateSecurityRig wires a fresh provider client + user, env
// with LogCapture enabled, and a connector pointing at the provider. No
// HTTP server is started — every case delivers `/oauth2/callback`
// in-process via env.PublicGin, which exercises the same handler chain.
func newCallbackStateSecurityRig(t *testing.T, name string) *callbackStateSecurityRig {
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

	return &callbackStateSecurityRig{
		provider:     provider,
		env:          env,
		logCapture:   logCapture,
		clientKey:    clientKey,
		userID:       user.ID,
		connectorID:  connectorID,
		connector:    connector,
		scopes:       []string{"read"},
		returnToURL:  "https://example.com/return",
		errorPageURL: env.Cfg.GetRoot().ErrorPages.InternalError,
	}
}

// initiateAndStateID kicks off a connection and returns the connection
// ID plus the state UUID the proxy persisted in Redis. The state UUID
// rides in the `state_id` query of the redirect URL.
func (r *callbackStateSecurityRig) initiateAndStateID(t *testing.T) (connectionID, stateID string) {
	t.Helper()
	connID, redirectURL := r.env.InitiateOAuth2Connection(t, r.connectorID, r.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID = parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID, "InitiateOAuth2Connection should embed state_id in redirect URL: %s", redirectURL)
	return connID, stateID
}

// requireNoToken asserts no oauth2_token row exists for the connection.
// Used in cases where a connection was created but the callback was
// rejected — no exchange should have been attempted.
func (r *callbackStateSecurityRig) requireNoToken(t *testing.T, connectionID string) {
	t.Helper()
	require.Nil(t, r.env.GetOAuth2Token(t, connectionID), "no oauth2_token row should exist when callback was rejected")
}

// requireConnectionUnchanged asserts the connection sat in `created`
// with no setup_step transition — proves the rejection short-circuited
// before HandleAuthFailed/HandleCredentialsEstablished ran.
func (r *callbackStateSecurityRig) requireConnectionUnchanged(t *testing.T, connectionID string) {
	t.Helper()
	conn := r.env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateSetup, conn.State,
		"connection state should remain `created` after a rejected callback")
	assert.Nil(t, conn.SetupStep, "no setup_step should be recorded on a rejected callback")
	assert.Nil(t, conn.SetupError, "no setup_error should be recorded on a rejected callback")
}

// requireOneRejection asserts exactly one rejection event was emitted
// with the given category. The single-event check is intentional: the
// event is the security alert, and double-emission would skew alerting.
func (r *callbackStateSecurityRig) requireOneRejection(t *testing.T, category string) map[string]any {
	t.Helper()
	events := r.logCapture.RecordsWithMessage(t, rejectionEventMessage)
	require.Lenf(t, events, 1, "expected exactly one rejection event; got %d (%v)", len(events), events)
	assert.Equal(t, category, events[0]["category"], "rejection category mismatch")
	return events[0]
}

// requireRejectionRedirect asserts the callback's 302 Location header
// points at the configured error page URL (not the connection's
// return-to URL).
func (r *callbackStateSecurityRig) requireRejectionRedirect(t *testing.T, location string) {
	t.Helper()
	require.NotEmpty(t, r.errorPageURL, "test config must set error_pages.internal_error")
	assert.Equal(t, r.errorPageURL, location,
		"callback should redirect to error_pages.internal_error on rejection; got %q", location)
}

// requireNoTokenCallObserved asserts the OAuth provider's /token
// endpoint saw zero requests during this test. Proves the token
// exchange path was short-circuited by state validation.
func (r *callbackStateSecurityRig) requireNoTokenCallObserved(t *testing.T) {
	t.Helper()
	tokenReqs := r.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: r.clientKey,
	})
	assert.Empty(t, tokenReqs, "provider must not have observed a /token call when callback was rejected")
}

func TestCallbackRejection_MissingState(t *testing.T) {
	rig := newCallbackStateSecurityRig(t, "missing-state")

	// No state, no connection — the callback handler bounces before any
	// Redis or DB access.
	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL("", "fake-code"))
	rig.requireRejectionRedirect(t, loc)

	event := rig.requireOneRejection(t, "missing_state")
	// missing_state rejects pre-parse, so no state_id should be attached.
	_, hasStateId := event["state_id"]
	assert.False(t, hasStateId, "missing_state event must not carry a state_id")

	rig.requireNoTokenCallObserved(t)
}

func TestCallbackRejection_UnknownState(t *testing.T) {
	rig := newCallbackStateSecurityRig(t, "unknown-state")

	// A valid-looking but never-persisted state id. apid prefix matters
	// for parse, so use the real oauth2-state prefix.
	bogusStateID := apid.New(apid.PrefixOauth2State).String()

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(bogusStateID, "fake-code"))
	rig.requireRejectionRedirect(t, loc)

	event := rig.requireOneRejection(t, "unknown_state")
	assert.Equal(t, bogusStateID, event["state_id"], "unknown_state event should record the parsed state_id")

	rig.requireNoTokenCallObserved(t)
}

func TestCallbackRejection_TamperedState(t *testing.T) {
	rig := newCallbackStateSecurityRig(t, "tampered-state")

	connID, stateID := rig.initiateAndStateID(t)

	// Read the encrypted envelope, flip a byte in the ciphertext, write
	// it back. AES-GCM's authentication tag fails on read, so the proxy
	// rejects with `tampered_state`.
	r := rig.env.DM.GetRedisClient()
	stateKey := "oauth2:state:" + stateID
	raw, err := r.Get(context.Background(), stateKey).Result()
	require.NoError(t, err, "state should be in Redis after _initiate")

	ef, err := encfield.ParseInlineString(raw)
	require.NoError(t, err)
	ciphertext, err := base64.StdEncoding.DecodeString(ef.Data)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	ciphertext[len(ciphertext)/2] ^= 0xff
	tampered := encfield.EncryptedField{ID: ef.ID, Data: base64.StdEncoding.EncodeToString(ciphertext)}
	require.NoError(t, r.Set(context.Background(), stateKey, tampered.ToInlineString(), time.Minute).Err())

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, "fake-code"))
	rig.requireRejectionRedirect(t, loc)

	event := rig.requireOneRejection(t, "tampered_state")
	assert.Equal(t, stateID, event["state_id"])

	rig.requireNoToken(t, connID)
	rig.requireConnectionUnchanged(t, connID)
	rig.requireNoTokenCallObserved(t)
}

func TestCallbackRejection_ReplayedState(t *testing.T) {
	rig := newCallbackStateSecurityRig(t, "replayed-state")

	connID, stateID := rig.initiateAndStateID(t)

	// Drive the consent leg programmatically through the test provider's
	// /test/authorize endpoint instead of a browser. The provider returns
	// the redirect URL it would have produced after the user clicked Allow,
	// from which we extract the real `code`.
	authResp := rig.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    rig.clientKey,
		UserID:      rig.userID,
		RedirectURI: rig.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(rig.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	require.NotEmpty(t, authResp.RedirectURL)
	providerCallback, parseErr := url.Parse(authResp.RedirectURL)
	require.NoError(t, parseErr)
	code := providerCallback.Query().Get("code")
	require.NotEmpty(t, code, "provider should issue a code on approve; got %s", authResp.RedirectURL)

	callbackURL := rig.env.ForgeOAuth2CallbackURL(stateID, code)

	// First delivery: state validates, code exchanged, state deleted from Redis.
	// Land on the connection's return_to_url (possibly with setup=pending suffix).
	loc := rig.env.DeliverOAuth2Callback(t, callbackURL)
	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"first callback should land on return_to_url; got %q", loc)
	require.Empty(t, rig.logCapture.RecordsWithMessage(t, rejectionEventMessage),
		"first callback must not emit a rejection event")

	// Replay: deliver the same callback URL a second time. The state was
	// deleted on consume, so the lookup misses and the proxy rejects with
	// `unknown_state`.
	loc = rig.env.DeliverOAuth2Callback(t, callbackURL)
	rig.requireRejectionRedirect(t, loc)

	event := rig.requireOneRejection(t, "unknown_state")
	assert.Equal(t, stateID, event["state_id"], "replay event should reflect the consumed state_id")

	// Connection has a token from the first (successful) callback — the
	// replay is just a no-op on top, neither corrupting nor dropping it.
	tok := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, tok, "first callback should have persisted a token row")
	assert.False(t, tok.EncryptedAccessToken.IsZero())

	// The provider observed exactly one /token call — for the first
	// callback. The replay must not have triggered another exchange.
	tokenReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: rig.clientKey,
	})
	require.Lenf(t, tokenReqs, 1, "provider should have observed exactly one /token call (the first callback); got %d", len(tokenReqs))
}
