package helpers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/stretchr/testify/require"
)

// OAuth2Option configures an OAuth2 helper invocation. The same option type
// is shared across the OAuth2 helpers (InitiateOAuth2Connection,
// DeliverOAuth2Callback, …) so callers learn one pattern. Each helper applies
// the fields that are meaningful to it and ignores the rest.
type OAuth2Option func(*oauth2Options)

// oauth2Options is the resolved configuration produced by applying the
// caller-supplied OAuth2Option values on top of the defaults each helper
// installs (default actor "test-actor" in the root namespace).
type oauth2Options struct {
	actorExternalID string
	actorNamespace  string
}

func (env *IntegrationTestEnv) resolveOAuth2Options(opts []OAuth2Option) oauth2Options {
	cfg := oauth2Options{
		actorExternalID: "test-actor",
		actorNamespace:  sconfig.RootNamespace,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithActor signs the OAuth2 helper request as the named actor in the given
// namespace. Mirrors the JWT a real browser session for that actor would
// carry. Defaults are actor "test-actor" in sconfig.RootNamespace.
//
// Caller is responsible for ensuring a non-root namespace already exists
// (see env.Core.CreateNamespace) before signing into it.
func WithActor(externalID, namespace string) OAuth2Option {
	return func(c *oauth2Options) {
		c.actorExternalID = externalID
		c.actorNamespace = namespace
	}
}

// OAuth2ConnectorOptions configures NewOAuth2Connector. Endpoints default to
// the test provider's URLs (provider.AuthorizationEndpoint(),
// provider.TokenEndpoint(), provider.RevocationEndpoint()) when zero.
type OAuth2ConnectorOptions struct {
	ClientID     string
	ClientSecret string
	// Scopes lists scope IDs that are required (Scope.Required defaults to
	// true via IsRequired when unspecified).
	Scopes []string
	// OptionalScopes lists scope IDs marked as optional (Required=false).
	// They are appended after Scopes in the connector definition.
	OptionalScopes []string
	// IncludeRevocation, when true, fills in the standard /v1/oauth/revoke
	// endpoint so revoke flows are exercised.
	IncludeRevocation bool
	// AuthorizationEndpoint overrides provider.AuthorizationEndpoint().
	AuthorizationEndpoint string
	// TokenEndpoint overrides provider.TokenEndpoint().
	TokenEndpoint string
	// RevocationEndpoint overrides provider.RevocationEndpoint() (only used
	// when IncludeRevocation is true).
	RevocationEndpoint string
}

// NewOAuth2Connector builds an authproxy connector wired to the given
// OAuth2TestProvider. The endpoints default to the provider's standard
// /v1/oauth/* URLs.
func NewOAuth2Connector(connectorID apid.ID, displayName string, provider *OAuth2TestProvider, opts OAuth2ConnectorOptions) sconfig.Connector {
	authEndpoint := opts.AuthorizationEndpoint
	if authEndpoint == "" {
		authEndpoint = provider.AuthorizationEndpoint()
	}
	tokenEndpoint := opts.TokenEndpoint
	if tokenEndpoint == "" {
		tokenEndpoint = provider.TokenEndpoint()
	}

	scopes := make([]connectors.Scope, 0, len(opts.Scopes)+len(opts.OptionalScopes))
	for _, id := range opts.Scopes {
		scopes = append(scopes, connectors.Scope{Id: id, Reason: "integration test"})
	}
	notRequired := false
	for _, id := range opts.OptionalScopes {
		scopes = append(scopes, connectors.Scope{Id: id, Required: &notRequired, Reason: "integration test"})
	}

	auth := &connectors.AuthOAuth2{
		Type:         connectors.AuthTypeOAuth2,
		ClientId:     &common.StringValue{InnerVal: &common.StringValueDirect{Value: opts.ClientID}},
		ClientSecret: &common.StringValue{InnerVal: &common.StringValueDirect{Value: opts.ClientSecret}},
		Scopes:       scopes,
		Authorization: connectors.AuthOauth2Authorization{
			Endpoint: authEndpoint,
		},
		Token: connectors.AuthOauth2Token{
			Endpoint: tokenEndpoint,
		},
	}

	if opts.IncludeRevocation {
		revocationEndpoint := opts.RevocationEndpoint
		if revocationEndpoint == "" {
			revocationEndpoint = provider.RevocationEndpoint()
		}
		auth.Revocation = &connectors.AuthOauth2Revocation{
			Endpoint: revocationEndpoint,
		}
	}

	return sconfig.Connector{
		Id:          connectorID,
		Version:     1,
		Labels:      map[string]string{"type": displayName},
		DisplayName: displayName,
		Auth: &connectors.Auth{
			InnerVal: auth,
		},
	}
}

// PublicOAuthCallbackURL returns the URL the proxy emits as the OAuth `redirect_uri`.
// The OAuth provider must be configured with this exact URL so authorize matches
// (helper exists because public.GetBaseUrl() depends on resolved config — port 0
// in integration.yaml means the URL is "http://localhost:0/oauth2/callback").
func (env *IntegrationTestEnv) PublicOAuthCallbackURL() string {
	return env.Cfg.GetRoot().Public.GetBaseUrl() + "/oauth2/callback"
}

// ForgeOAuth2CallbackURL builds a `/oauth2/callback?...` URL with the
// state and code values provided. Used by the callback-state-security
// tests to construct callbacks the browser would never produce on its
// own (missing state, unknown state, replayed state, etc.). state and
// code may be empty.
func (env *IntegrationTestEnv) ForgeOAuth2CallbackURL(state, code string) string {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if code != "" {
		q.Set("code", code)
	}
	base := env.PublicOAuthCallbackURL()
	if encoded := q.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

// InitiateOAuth2Connection POSTs to /api/v1/connections/_initiate and returns the
// new connection's ID and the redirect URL pointing at the public service's
// /oauth2/redirect endpoint. Works in both in-process (ApiGin) and real HTTP
// (ServerURL) modes. By default the request is signed as actor "test-actor" in
// the root namespace; pass WithActor(...) for multi-actor or multi-tenant
// scenarios.
//
// Caller is responsible for ensuring the actor's namespace exists (via
// env.Core.CreateNamespace or its ancestors) when it is not the root namespace.
func (env *IntegrationTestEnv) InitiateOAuth2Connection(t *testing.T, connectorID apid.ID, returnToUrl string, opts ...OAuth2Option) (connectionID, redirectURL string) {
	t.Helper()
	require.Truef(t, env.ApiGin != nil || env.ServerURL != "",
		"InitiateOAuth2Connection requires either in-process gin or a running HTTP server")

	cfg := env.resolveOAuth2Options(opts)

	// Land the connection in the actor's namespace by default. Without
	// IntoNamespace, the API places the connection in the connector's
	// namespace (root), which doesn't reflect how a real multi-tenant
	// deployment isolates tenants — and breaks state-vs-connection
	// namespace checks the security tests rely on.
	body, err := jsonMarshal(coreIface.InitiateConnectionRequest{
		ConnectorId:   connectorID,
		ReturnToUrl:   returnToUrl,
		IntoNamespace: cfg.actorNamespace,
	})
	require.NoError(t, err)

	const path = "/api/v1/connections/_initiate"
	req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodPost,
		path,
		body,
		cfg.actorNamespace,
		cfg.actorExternalID,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	if env.ApiGin != nil {
		env.ApiGin.ServeHTTP(w, req)
	} else {
		abs, err := url.Parse(env.ServerURL + path)
		require.NoError(t, err)
		req.URL = abs
		req.Host = abs.Host
		req.RequestURI = ""
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		w.Code = resp.StatusCode
		if _, err := w.Body.ReadFrom(resp.Body); err != nil {
			require.NoError(t, err)
		}
	}
	require.Equalf(t, http.StatusOK, w.Code, "initiate failed: %s", w.Body.String())

	var resp coreIface.ConnectionSetupRedirect
	require.NoError(t, jsonUnmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, coreIface.ConnectionSetupResponseTypeRedirect, resp.Type, "expected OAuth2 connector to return redirect")
	require.NotEmpty(t, resp.RedirectUrl)

	return resp.Id.String(), resp.RedirectUrl
}

// FollowOAuth2Redirect issues an in-process GET to the public service's
// `/oauth2/redirect` endpoint with the same state_id and signed JWT the user's
// browser would carry, and returns the Location header — the URL of the OAuth
// provider's authorize endpoint. The proxy generates the upstream URL from the
// connector config, so callers can assert on its query parameters.
func (env *IntegrationTestEnv) FollowOAuth2Redirect(t *testing.T, redirectURL string) string {
	t.Helper()
	require.NotNil(t, env.PublicGin, "FollowOAuth2Redirect requires SetupOptions.IncludePublic=true")

	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)

	req, err := env.PublicAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		parsed.RequestURI(),
		nil,
		sconfig.RootNamespace,
		"test-actor",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.PublicGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusFound, w.Code, "/oauth2/redirect should 302; got %d body=%s", w.Code, w.Body.String())

	loc := w.Header().Get("Location")
	require.NotEmpty(t, loc, "/oauth2/redirect should set Location")
	return loc
}

// DeliverOAuth2Callback issues an in-process GET to the public service's
// `/oauth2/callback` endpoint and returns the final Location header — typically
// the test's return_to_url on success, or error_pages.internal_error on
// rejection. By default the request is signed as actor "test-actor" in the root
// namespace; pass WithActor(...) to mirror a specific tenant's browser session.
func (env *IntegrationTestEnv) DeliverOAuth2Callback(t *testing.T, callbackURL string, opts ...OAuth2Option) string {
	t.Helper()
	require.NotNil(t, env.PublicGin, "DeliverOAuth2Callback requires SetupOptions.IncludePublic=true")

	cfg := env.resolveOAuth2Options(opts)

	parsed, err := url.Parse(callbackURL)
	require.NoError(t, err)

	req, err := env.PublicAuthUtil.NewSignedRequestForActorExternalId(
		http.MethodGet,
		parsed.RequestURI(),
		nil,
		cfg.actorNamespace,
		cfg.actorExternalID,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	env.PublicGin.ServeHTTP(w, req)
	require.Equalf(t, http.StatusFound, w.Code, "/oauth2/callback should 302; got %d body=%s", w.Code, w.Body.String())

	loc := w.Header().Get("Location")
	require.NotEmpty(t, loc, "/oauth2/callback should set Location")
	return loc
}

// OAuth2StateForTest mirrors the unexported `state` struct in
// internal/auth_methods/oauth2 (see state.go:24-34) so tests can construct
// synthetic state envelopes that exercise validation paths the natural
// _initiate flow can't reach (e.g., crafted namespace/actor mismatches
// against the state-vs-caller and state-vs-connection checks).
//
// JSON tags must stay in sync with the production struct; if they drift,
// the production decode will fail with errStateTampered.
type OAuth2StateForTest struct {
	Id                     apid.ID   `json:"id"`
	Namespace              string    `json:"namespace"`
	ActorId                apid.ID   `json:"actor_id"`
	ConnectorId            apid.ID   `json:"connector_id"`
	ConnectorVersion       uint64    `json:"connector_version"`
	ConnectionId           apid.ID   `json:"connection_id"`
	ReturnToUrl            string    `json:"return_to"`
	CancelSessionAfterAuth bool      `json:"cancel_session_after_auth"`
	ExpiresAt              time.Time `json:"expires_at"`
}

// WriteOAuth2StateForTest encrypts and stores a synthetic OAuth2 state
// envelope in Redis at the production key `oauth2:state:<state.Id>`,
// using env.DM's encrypt service so the envelope round-trips through the
// same AEAD as a state minted by the real `_initiate` flow.
//
// Used by tests that need the production callback handler to read a state
// envelope with hand-crafted fields (namespace_mismatch_actor,
// namespace_mismatch_connection, etc.) — values the natural flow would
// never produce.
func (env *IntegrationTestEnv) WriteOAuth2StateForTest(t *testing.T, s OAuth2StateForTest, ttl time.Duration) {
	t.Helper()
	plaintext, err := json.Marshal(s)
	require.NoError(t, err)
	ctx := context.Background()
	ef, err := env.DM.GetEncryptService().EncryptGlobal(ctx, plaintext)
	require.NoError(t, err)
	key := fmt.Sprintf("oauth2:state:%s", s.Id.String())
	require.NoError(t, env.DM.GetRedisClient().Set(ctx, key, ef.ToInlineString(), ttl).Err())
}

// GetOAuth2Token reads the most recent OAuth2 token row stored for the
// connection. Returns nil when no token exists yet.
func (env *IntegrationTestEnv) GetOAuth2Token(t *testing.T, connectionID string) *database.OAuth2Token {
	t.Helper()
	id, err := apid.Parse(connectionID)
	require.NoError(t, err)
	tok, err := env.Db.GetOAuth2Token(context.Background(), id)
	if errors.Is(err, database.ErrNotFound) {
		return nil
	}
	require.NoError(t, err)
	return tok
}

// DecryptOAuth2RefreshToken decrypts the persisted refresh_token for the
// connection's current token row and returns the plaintext. Tests that need
// to assert on the *value* of the refresh token (e.g. rotation tests) must
// decrypt because EncryptedField bytes change on every encryption (AES-GCM
// uses a fresh nonce), so equality of encrypted bytes is not a reliable
// signal that the plaintext changed.
func (env *IntegrationTestEnv) DecryptOAuth2RefreshToken(t *testing.T, tok *database.OAuth2Token) string {
	t.Helper()
	require.NotNil(t, tok, "DecryptOAuth2RefreshToken: token must not be nil")
	require.False(t, tok.EncryptedRefreshToken.IsZero(),
		"DecryptOAuth2RefreshToken: token has no encrypted refresh_token")
	plaintext, err := env.DM.GetEncryptService().DecryptString(context.Background(), tok.EncryptedRefreshToken)
	require.NoError(t, err, "decrypt refresh_token")
	return plaintext
}
