//go:build integration

package oauth2

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
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

// pkceVerifierAlphabet mirrors RFC 7636 §4.1 — the production generator is
// internal to the oauth2 package, so the integration tests pin the alphabet
// here to assert the verifier the proxy persists is well-formed without
// reaching into unexported helpers.
const pkceVerifierAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// minVerifierLen is RFC 7636 §4.1's floor; the production generator emits
// exactly 43 chars but the spec allows 43-128, so the assertion accepts the
// full range — the test cares that the verifier is conforming, not that
// it's exactly the length the current generator picked.
const (
	minVerifierLen = 43
	maxVerifierLen = 128
)

// pkceS256Challenge computes base64url(sha256(verifier)) with no padding —
// the RFC 7636 §4.2 S256 transformation. Duplicated here (rather than
// imported from internal/auth_methods/oauth2) because integration tests
// should not depend on package-internal helpers; matching values prove the
// two implementations agree on the contract.
func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// freshVerifier mints a 43-char verifier from the RFC 7636 unreserved
// alphabet for the MismatchedVerifier subtest. Distinct from the production
// generator so test failures don't shadow real bugs.
func freshVerifier(t *testing.T) string {
	t.Helper()
	const n = 43
	buf := make([]byte, n)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = pkceVerifierAlphabet[int(b)%len(pkceVerifierAlphabet)]
	}
	return string(out)
}

// requireVerifierShape pins the RFC 7636 §4.1 invariants on a verifier read
// from Redis (the encrypted state record stores the plaintext verifier;
// only the provider's request recorder redacts it on the wire). Length
// within [43,128] and every byte from the unreserved alphabet. A verifier
// outside these bounds would be rejected by spec-conformant providers.
func requireVerifierShape(t *testing.T, verifier string) {
	t.Helper()
	require.GreaterOrEqualf(t, len(verifier), minVerifierLen,
		"persisted verifier shorter than RFC 7636 §4.1 minimum: %q", verifier)
	require.LessOrEqualf(t, len(verifier), maxVerifierLen,
		"persisted verifier longer than RFC 7636 §4.1 maximum: %q", verifier)
	for i := 0; i < len(verifier); i++ {
		require.True(t, strings.IndexByte(pkceVerifierAlphabet, verifier[i]) >= 0,
			"persisted verifier contains illegal byte %q at idx %d (%q)", verifier[i], i, verifier)
	}
}

// hasCodeVerifierForm reports whether the recorded /token form carried a
// non-empty `code_verifier`. The test provider redacts the verifier value
// to "<redacted>" in its recorder (per its sensitive-field policy), so
// integration tests check presence-vs-absence, not the literal value. The
// literal-value contract is proved via the pre-callback Redis state read
// + S256 hash equality against the authorize URL's challenge.
func hasCodeVerifierForm(form map[string][]string) bool {
	v, ok := form["code_verifier"]
	if !ok {
		return false
	}
	for _, x := range v {
		if x != "" {
			return true
		}
	}
	return false
}

// TestPKCE_HappyPath_S256 drives the full PKCE-enabled authorization-code
// flow through the marketplace UI against a provider client registered with
// RequirePKCE: true. A green flow proves the proxy emits the challenge on
// authorize and the matching verifier on token exchange — the provider
// would reject both halves otherwise.
//
// See pkce_test.md for the scenario specification.
func TestPKCE_HappyPath_S256(t *testing.T) {
	provider := helpers.NewOAuth2TestProvider(t)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	clientKey := "pkce-happy-client-" + suffix
	clientSecret := "pkce-happy-secret-" + suffix
	userPassword := "p4ssw0rd-" + suffix
	userEmail := "alice-pkce-" + suffix + "@example.com"

	connectorID := apid.New(apid.PrefixConnectorVersion)
	connector := helpers.NewOAuth2Connector(connectorID, "pkce-happy-test", provider, helpers.OAuth2ConnectorOptions{
		ClientID:     clientKey,
		ClientSecret: clientSecret,
		Scopes:       []string{"read"},
		PKCE:         &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
	})

	env := helpers.Setup(t, helpers.SetupOptions{
		Service:            helpers.ServiceTypeAPI,
		StartHTTPServer:    true,
		IncludePublic:      true,
		ServeMarketplaceUI: true,
		Connectors:         []sconfig.Connector{connector},
	})
	defer env.Cleanup()

	// RequirePKCE: true makes the test provider refuse authorize without
	// code_challenge — a missing challenge would 4xx at /web/authorize, the
	// chromedp wait would time out, and the test would fail before any of
	// the substantive PKCE assertions run.
	registered := provider.CreateClient(helpers.CreateClientRequest{
		Key:                     clientKey,
		Secret:                  clientSecret,
		RedirectURI:             env.PublicOAuthCallbackURL(),
		TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
		Scope:                   "read",
		RequirePKCE:             true,
	})
	require.Equal(t, clientKey, registered.Key)
	require.True(t, registered.RequirePKCE, "provider should echo back RequirePKCE=true on the registered client")

	user := provider.CreateUser(helpers.CreateUserRequest{
		Username: userEmail,
		Password: userPassword,
		Email:    userEmail,
	})
	require.NotEmpty(t, user.ID)

	authToken, err := env.PublicAuthUtil.GenerateBearerToken(
		context.Background(),
		"test-actor",
		sconfig.RootNamespace,
		aschema.AllPermissions(),
	)
	require.NoError(t, err)

	browserCtx, _ := helpers.NewBrowser(t)

	connectorsURL := env.PublicURL + "/connectors?auth_token=" + url.QueryEscape(authToken)
	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Navigate(connectorsURL),
		chromedp.WaitVisible(`//button[normalize-space()='Connect']`, chromedp.BySearch),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`//button[normalize-space()='Connect']`, chromedp.BySearch),
		chromedp.WaitVisible(`input[name="email"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.SendKeys(`input[name="email"]`, userEmail, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, userPassword, chromedp.ByQuery),
		chromedp.Submit(`input[name="email"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="allow"]`, chromedp.ByQuery),
	))

	require.NoError(t, chromedp.Run(browserCtx,
		chromedp.Click(`input[name="allow"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`//h1[normalize-space()='Your Connections']`, chromedp.BySearch),
	))

	// Locate the new connection. We don't read state from Redis here — the
	// callback handler deletes it after consuming the code, so any read
	// after WaitVisible above would race the cleanup. Instead we assert on
	// what the provider saw (challenge on authorize, verifier on token).
	page := env.Db.ListConnectionsBuilder().
		ForNamespaceMatcher(sconfig.RootNamespace).
		Limit(10).
		FetchPage(context.Background())
	require.NoError(t, page.Error)
	require.Lenf(t, page.Results, 1, "expected exactly one connection after PKCE flow; got %d", len(page.Results))
	connectionID := page.Results[0].Id.String()

	// Token persisted and not expired — the proxy must have parsed a
	// success response from the provider, which only returns 200 when
	// PKCE verifies.
	token := env.GetOAuth2Token(t, connectionID)
	require.NotNil(t, token, "PKCE success path must persist an OAuth2 token row")
	assert.False(t, token.EncryptedAccessToken.IsZero(), "access token must be stored encrypted")
	require.NotNil(t, token.AccessTokenExpiresAt)
	assert.True(t, token.AccessTokenExpiresAt.After(time.Now()),
		"PKCE success path token should not be pre-expired (expires_at=%s)", token.AccessTokenExpiresAt)

	conn := env.GetConnection(t, connectionID)
	assert.Equal(t, database.ConnectionStateReady, conn.State,
		"PKCE success path should land the connection in ready")

	// Provider must have observed a /token POST carrying code_verifier.
	// The recorder redacts the literal value (see hasCodeVerifierForm
	// docstring), so the strongest contract we can prove here is
	// "present and non-empty" — but a missing or stripped verifier
	// would fail the provider's PKCE check, and that failure would
	// already have surfaced as a 4xx blocking the chromedp wait above.
	// The combination of "field present in form" + "flow completed
	// successfully" + "client was registered with RequirePKCE: true"
	// is the literal-value contract one step removed.
	tokenReqs := provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: clientKey,
	})
	require.Lenf(t, tokenReqs, 1, "expected exactly one /token POST; got %d", len(tokenReqs))
	tokenForm := tokenReqs[0].Form
	assert.Truef(t, hasCodeVerifierForm(tokenForm),
		"PKCE-enabled token exchange must include code_verifier; form keys=%v",
		formKeys(tokenForm))
	assert.Equal(t, "authorization_code", lastForm(tokenForm, "grant_type"))
}

// formKeys returns the sorted set of keys present in a recorded form, for
// error messages that need to convey "what fields were sent" without
// leaking redacted bodies.
func formKeys(form map[string][]string) []string {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	return keys
}

// TestPKCE_TokenExchangeRejected covers issue #174's "missing" and
// "invalid" code_verifier cases by mutating the persisted state record
// between authorize and callback. Both subtests share the rig and differ
// only in how they mutate the verifier — empty for missing, fresh for
// mismatched.
//
// The state-mutation approach is necessary because a correctly wired
// proxy will never emit a missing or mismatched verifier on its own —
// both come from forging the state.
func TestPKCE_TokenExchangeRejected(t *testing.T) {
	subtests := []struct {
		name string
		// mutate returns the verifier that should end up in state. Empty
		// string means "clear the verifier" (the MissingVerifier case);
		// any other value means "replace it" (MismatchedVerifier).
		mutate func(t *testing.T, original string) string
		// expectVerifierSent is what the test asserts the proxy POSTed to
		// /token. For MissingVerifier the form key is absent; for
		// MismatchedVerifier the form key matches the mutated value.
		expectVerifierAbsent bool
	}{
		{
			name: "MissingVerifier",
			mutate: func(t *testing.T, _ string) string {
				return ""
			},
			expectVerifierAbsent: true,
		},
		{
			name: "MismatchedVerifier",
			mutate: func(t *testing.T, original string) string {
				v := freshVerifier(t)
				require.NotEqualf(t, original, v,
					"mutated verifier must differ from the original; both were %q", v)
				return v
			},
			expectVerifierAbsent: false,
		},
	}

	for _, sub := range subtests {
		t.Run(sub.name, func(t *testing.T) {
			provider := helpers.NewOAuth2TestProvider(t)

			suffix := fmt.Sprintf("%d", time.Now().UnixNano())
			clientKey := "pkce-rej-" + strings.ToLower(sub.name) + "-" + suffix
			clientSecret := "pkce-rej-secret-" + suffix
			userEmail := "alice-pkce-rej-" + suffix + "@example.com"
			returnToURL := "https://example.com/return"

			connectorID := apid.New(apid.PrefixConnectorVersion)
			connector := helpers.NewOAuth2Connector(connectorID, "pkce-rejected", provider, helpers.OAuth2ConnectorOptions{
				ClientID:     clientKey,
				ClientSecret: clientSecret,
				Scopes:       []string{"read"},
				PKCE:         &cschema.AuthOauth2PKCE{Method: cschema.PKCEMethodS256},
			})

			logCapture := helpers.NewLogCapture()
			env := helpers.Setup(t, helpers.SetupOptions{
				Service:       helpers.ServiceTypeAPI,
				IncludePublic: true,
				Connectors:    []sconfig.Connector{connector},
				LogCapture:    logCapture,
			})
			defer env.Cleanup()

			provider.CreateClient(helpers.CreateClientRequest{
				Key:                     clientKey,
				Secret:                  clientSecret,
				RedirectURI:             env.PublicOAuthCallbackURL(),
				TokenEndpointAuthMethod: helpers.TokenEndpointAuthPost,
				Scope:                   "read",
				RequirePKCE:             true,
			})
			user := provider.CreateUser(helpers.CreateUserRequest{
				Username: userEmail,
				Password: "irrelevant-test-password",
				Email:    userEmail,
			})

			// Step 1: initiate. The state record is written to Redis with a
			// freshly generated verifier (because the connector has a PKCE
			// block); the redirect URL points at /oauth2/redirect with the
			// new state_id.
			connID, redirectURL := env.InitiateOAuth2Connection(t, connectorID, returnToURL)

			redirectParsed, err := url.Parse(redirectURL)
			require.NoError(t, err)
			stateIDStr := redirectParsed.Query().Get("state_id")
			require.NotEmpty(t, stateIDStr, "InitiateOAuth2Connection should embed state_id: %s", redirectURL)
			stateID, err := apid.Parse(stateIDStr)
			require.NoError(t, err)

			// Step 2: follow the proxy's redirect to build the upstream
			// authorize URL. The proxy emits code_challenge into this URL
			// when the connector has a PKCE block — exercising the
			// authorize-side leg.
			upstreamAuthorizeURL := env.FollowOAuth2Redirect(t, redirectURL)
			upstreamParsed, err := url.Parse(upstreamAuthorizeURL)
			require.NoError(t, err)
			challenge := upstreamParsed.Query().Get("code_challenge")
			require.NotEmptyf(t, challenge,
				"PKCE-enabled connector must emit code_challenge in the upstream authorize URL; got %s", upstreamAuthorizeURL)
			require.Equal(t, "S256", upstreamParsed.Query().Get("code_challenge_method"),
				"connector configured with method=S256 must emit code_challenge_method=S256")

			// Snapshot the original verifier so MismatchedVerifier can assert
			// "the mutated value, not the original, was POSTed" — proving the
			// proxy reads verifier from state fresh on each call rather than
			// caching it from authorize.
			originalState := env.ReadOAuth2StateForTest(t, stateID)
			require.NotEmpty(t, originalState.PKCECodeVerifier,
				"natural _initiate with PKCE block must persist a non-empty verifier")
			require.Equal(t, cschema.PKCEMethodS256, originalState.PKCEMethod)
			require.Equal(t, challenge, pkceS256Challenge(originalState.PKCECodeVerifier),
				"persisted verifier must hash to the challenge emitted on authorize")

			// Step 3: mint a real code via /test/authorize, passing the
			// challenge so the provider binds the code to the original
			// challenge. After this step, the provider's PKCE check will
			// compare anything the proxy sends at /token against `challenge`.
			authResp := provider.Authorize(helpers.AuthorizeRequest{
				ClientID:            clientKey,
				UserID:              user.ID,
				RedirectURI:         env.PublicOAuthCallbackURL(),
				Scope:               "read",
				State:               stateIDStr,
				Decision:            helpers.AuthorizeApprove,
				CodeChallenge:       challenge,
				CodeChallengeMethod: "S256",
			})
			require.NotEmpty(t, authResp.RedirectURL)
			authResultParsed, err := url.Parse(authResp.RedirectURL)
			require.NoError(t, err)
			code := authResultParsed.Query().Get("code")
			require.NotEmpty(t, code, "provider should issue a code on approve")

			// Step 4: forge the verifier mutation. The state record persists
			// in Redis at oauth2:state:<stateID>; replacing the verifier
			// before callback forces the proxy to send the mutated value (or
			// nothing, in the MissingVerifier case) on /token.
			mutated := sub.mutate(t, originalState.PKCECodeVerifier)
			forged := originalState
			forged.PKCECodeVerifier = mutated
			env.WriteOAuth2StateForTest(t, forged, 5*time.Minute)

			// Step 5: deliver the callback. The proxy reads the (mutated)
			// state, posts /token, and the provider rejects PKCE. The
			// failure path here mirrors any other token-exchange failure
			// during setup: redirect to return_to_url with setup=pending so
			// the marketplace UI re-renders the connection in its
			// auth_failed state (same shape scenario 7 pins for plain
			// invalid_grant cases).
			loc := env.DeliverOAuth2Callback(t, env.ForgeOAuth2CallbackURL(stateIDStr, code))
			require.Truef(t, strings.HasPrefix(loc, returnToURL),
				"PKCE rejection should redirect to return_to_url with setup=pending; got %q", loc)
			parsed, err := url.Parse(loc)
			require.NoError(t, err)
			assert.Equal(t, "pending", parsed.Query().Get("setup"),
				"PKCE rejection redirect should carry setup=pending")
			assert.Equal(t, connID, parsed.Query().Get("connection_id"),
				"PKCE rejection redirect should carry the connection_id")

			// Exactly one structured token-exchange failure event. The
			// category depends on whether the provider's error body
			// included `error=invalid_grant`. go-oauth2-server returns a
			// 4xx for PKCE failures but without a §5.2 `error` field, so
			// the proxy classifies the response as `provider_4xx_other`.
			// We accept either to stay robust against the provider
			// tightening its error body in a future version.
			events := logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage)
			require.Lenf(t, events, 1, "expected exactly one token-exchange failure event; got %d (%v)", len(events), events)
			category, _ := events[0]["category"].(string)
			assert.Containsf(t, []string{"invalid_grant", "provider_4xx_other"}, category,
				"PKCE rejection at /token must classify as invalid_grant or provider_4xx_other; got %q", category)
			if status, ok := events[0]["provider_status_code"].(float64); ok {
				assert.GreaterOrEqualf(t, int(status), 400,
					"provider_status_code should be a 4xx; got %v", status)
				assert.Lessf(t, int(status), 500,
					"provider_status_code should be a 4xx; got %v", status)
			}

			// No token row created — failed PKCE must not create a connection.
			assert.Nil(t, env.GetOAuth2Token(t, connID),
				"failed PKCE must not persist an OAuth2 token row")

			// Connection lands in the auth_failed terminal step. The state
			// stays `created` because the connection never reached the
			// credentials-established transition.
			connAfter := env.GetConnection(t, connID)
			assert.Equal(t, database.ConnectionStateCreated, connAfter.State,
				"connection state should remain `created` on PKCE rejection")
			require.NotNilf(t, connAfter.SetupStep,
				"PKCE-rejected connection should have a setup_step recorded")
			assert.Truef(t, connAfter.SetupStep.Equals(cschema.SetupStepAuthFailed),
				"connection should land in auth_failed; got %q", connAfter.SetupStep.String())
			require.NotNilf(t, connAfter.SetupError,
				"PKCE-rejected connection should have setup_error populated")

			// Exactly one /token call observed at the provider, with the
			// expected verifier presence/absence. The recorder redacts the
			// verifier value (see hasCodeVerifierForm docstring) — the
			// MismatchedVerifier subtest cannot pin the literal value on
			// the wire. The behavioral contract that the proxy did send
			// the mutated value (and not the original) is proved by the
			// provider's PKCE rejection itself: if the proxy had sent the
			// original verifier, the provider would have accepted it and
			// returned a valid token, and the rest of the assertions
			// above would have failed differently.
			tokenReqs := provider.Requests(helpers.RequestsFilter{
				Endpoint: helpers.EndpointToken,
				ClientID: clientKey,
			})
			require.Lenf(t, tokenReqs, 1, "expected exactly one /token POST; got %d", len(tokenReqs))
			form := tokenReqs[0].Form
			hasVerifier := hasCodeVerifierForm(form)
			if sub.expectVerifierAbsent {
				assert.Falsef(t, hasVerifier,
					"MissingVerifier subtest: proxy must not send code_verifier when state has none; form keys=%v",
					formKeys(form))
			} else {
				assert.Truef(t, hasVerifier,
					"MismatchedVerifier subtest: proxy must send (some) code_verifier from state; form keys=%v",
					formKeys(form))
			}
		})
	}
}
