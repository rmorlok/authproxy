//go:build integration

package oauth2

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
)

// Scenario 12 from issue #173: logs, errors, and metrics emitted by any
// OAuth flow must not contain access tokens, refresh tokens, authorization
// codes, client secrets, PKCE code verifiers, or raw provider credentials.
// Logs may include connection_id, tenant id, provider id, correlation id,
// error category, retry count, and scope-mismatch metadata.
//
// The proxy redacts at the wire layer (request_log) and at the structured
// log layer (each oauth2 log call lists explicit slog attributes — no
// %+v dumps of token responses). This file drives a representative subset
// of flows and asserts the redaction holds end-to-end.
//
// Approach
//
// The OAuth flow's secrets are *known to the test* — the client secret is
// configured locally, the authorization code comes back from
// provider.Authorize, and the access/refresh tokens are decrypted from the
// persisted DB row. After the flow, we serialize every captured log record
// back to JSON and check that none of those exact strings appear in the
// serialized text. This catches both attribute leaks (a slog.String key
// whose value contains the secret) and message-string leaks (an error
// message that includes the raw provider response).
//
// What this *doesn't* cover
//
//   - PKCE code verifiers: the proxy does not implement PKCE in the
//     codebase yet (issue #174 / scenario 14 is P1). When PKCE lands, this
//     test should add a code_verifier secret to the per-flow checks.
//   - Gin's HTTP access log line. Gin writes those directly to stdout,
//     not through the configured slog handler, so the LogCapture does
//     not see them. The access log only contains method + path + status +
//     latency — none of the listed sensitive values would appear there
//     either, but if the proxy ever changes to emit access logs through
//     slog, this test will start checking them automatically.
//   - The structured request_log (full HTTP transcripts persisted to
//     ClickHouse). request_log applies its own redaction layer; that's
//     tested in `internal/request_log`. The integration-test harness
//     doesn't wire request_log up by default, so this test cannot
//     exercise it from the boundary.
//
// PKCE code verifiers, raw provider credentials beyond client_secret, and
// any future secret types should be added to `flowSecrets` as the proxy
// gains the relevant features. Adding a new secret type without adding it
// here is the silent-leak failure mode this test is designed to catch.

// flowSecrets are the per-flow values whose plaintext must never appear in
// any captured log record. Each field is the raw value, not a substring —
// short or empty values are skipped because they'd false-positive against
// unrelated record content (e.g., a 4-char client_id colliding with a
// numeric attribute value).
type flowSecrets struct {
	ClientSecret      string
	AuthorizationCode string
	AccessTokenValue  string
	RefreshTokenValue string
	UserPassword      string
}

// assertNoSecretsInLogs serializes every captured record to JSON and
// asserts none of the supplied secrets appear as a substring. Values
// shorter than minSecretLen (or empty) are skipped — a 3-character
// client_id can easily collide with unrelated field content.
func assertNoSecretsInLogs(t *testing.T, capture *helpers.LogCapture, secrets flowSecrets) {
	t.Helper()

	const minSecretLen = 8

	records := capture.Records(t)
	require.NotEmptyf(t, records, "log capture has no records — fixture sanity check failed")

	// Re-serialize each record to JSON; we want a uniform haystack
	// regardless of slog attribute shape (string, int, nested map).
	joined := serializeRecords(t, records)

	check := func(label, value string) {
		t.Helper()
		if value == "" || len(value) < minSecretLen {
			return
		}
		assert.NotContainsf(t, joined, value,
			"%s plaintext must not appear in any captured log record (value=%q)", label, value)
	}

	check("client_secret", secrets.ClientSecret)
	check("authorization_code", secrets.AuthorizationCode)
	check("access_token", secrets.AccessTokenValue)
	check("refresh_token", secrets.RefreshTokenValue)
	check("user_password", secrets.UserPassword)
}

// serializeRecords concatenates the JSON form of every captured record.
// Re-encoding (rather than reusing the raw buffer) normalizes formatting
// and guarantees we scan the exact key/value text the slog handler would
// have written.
func serializeRecords(t *testing.T, records []map[string]any) string {
	t.Helper()
	var b strings.Builder
	enc := json.NewEncoder(&b)
	for _, r := range records {
		require.NoError(t, enc.Encode(r), "re-encode captured log record")
	}
	return b.String()
}

// assertConnectionIDPresent is a positive control: if logs from the OAuth
// flow are NOT being captured at all, the negative assertions in
// assertNoSecretsInLogs trivially pass. Pin a known-allowed value
// (connection_id) so the test fails loudly when the capture path
// regresses.
func assertConnectionIDPresent(t *testing.T, capture *helpers.LogCapture, connectionID string) {
	t.Helper()
	require.NotEmpty(t, connectionID, "positive control: connection_id is empty — test bug")
	joined := serializeRecords(t, capture.Records(t))
	require.Containsf(t, joined, connectionID,
		"positive control: connection_id %q should appear in some captured record — log capture may be misconfigured", connectionID)
}

// TestRedaction_HappyPathFlowLeaksNoSecrets — drive a complete authorization-
// code flow + one proxied API call. The resulting log stream covers
// initiate, callback (token exchange), and proxy request handling. None
// of the per-flow secrets must appear in any captured record.
func TestRedaction_HappyPathFlowLeaksNoSecrets(t *testing.T) {
	rig := newProxyRefreshRig(t, "redaction-happy")
	secrets := flowSecrets{}

	// completeAuthFlow uses the test provider's /test/authorize shortcut
	// (matches the rest of the refresh suite). We reproduce its work
	// inline here so we can capture the authorization code as it crosses
	// from the provider to the proxy — that exact code value is the
	// secret we need to assert never appears in logs.
	connID, redirectURL := rig.env.InitiateOAuth2Connection(t, rig.connectorID, rig.returnToURL)
	parsed, err := url.Parse(redirectURL)
	require.NoError(t, err)
	stateID := parsed.Query().Get("state_id")
	require.NotEmpty(t, stateID)

	authResp := rig.provider.Authorize(helpers.AuthorizeRequest{
		ClientID:    rig.clientKey,
		UserID:      rig.userID,
		RedirectURI: rig.env.PublicOAuthCallbackURL(),
		Scope:       strings.Join(rig.scopes, " "),
		State:       stateID,
		Decision:    helpers.AuthorizeApprove,
	})
	pc, err := url.Parse(authResp.RedirectURL)
	require.NoError(t, err)
	secrets.AuthorizationCode = pc.Query().Get("code")
	require.NotEmpty(t, secrets.AuthorizationCode, "fixture sanity: provider must mint a code")

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, secrets.AuthorizationCode))
	require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
		"auth flow must land on return_to_url; got %q", loc)

	// One proxied API call so proxy-path logs are exercised too.
	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.Equalf(t, 200, w.Code, "proxied request must succeed; got %d body=%s", w.Code, w.Body.String())

	// After persistence, decrypt the access + refresh tokens. Both are
	// raw provider-issued strings — if any code path logs them in
	// plaintext (e.g. by dumping the token response), the substring
	// check below will catch it.
	token := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token, "happy-path flow must persist a token row")
	secrets.AccessTokenValue = rig.env.DecryptOAuth2AccessToken(t, token)
	secrets.RefreshTokenValue = rig.env.DecryptOAuth2RefreshToken(t, token)

	// The client_secret is what the rig configures the connector with
	// — pinned to the rig's deterministic suffix so it's a known
	// substring to search for.
	secrets.ClientSecret = extractClientSecret(t, rig.clientKey)

	assertConnectionIDPresent(t, rig.logCapture, connID)
	assertNoSecretsInLogs(t, rig.logCapture, secrets)
}

// TestRedaction_TokenExchangeFailureLeaksNoSecrets — failure paths
// historically leak more than success paths: error messages tend to
// include the original request body and the provider response body, both
// of which carry secrets. Drive an `invalid_grant` token-exchange failure
// and assert nothing slipped through.
func TestRedaction_TokenExchangeFailureLeaksNoSecrets(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "redaction-token-exchange-fail")
	connID, stateID, code := rig.initiateAndMintCode(t)
	rig.scriptTokenEndpoint(helpers.ScriptAction{Status: 400, Body: rfc6749Error("invalid_grant")})

	loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
	rig.requireRedirectToReturnURL(t, connID, loc)
	// Sanity: the failure event was emitted so we know logs from the
	// failure path are in the capture.
	rig.requireOneFailureEvent(t, "invalid_grant")

	secrets := flowSecrets{
		ClientSecret:      extractClientSecret(t, rig.clientKey),
		AuthorizationCode: code,
	}

	assertConnectionIDPresent(t, rig.logCapture, connID)
	assertNoSecretsInLogs(t, rig.logCapture, secrets)
}

// TestRedaction_RefreshFailureLeaksNoSecrets — drive an invalid_grant
// refresh failure and assert no secrets leak. The refresh failure path
// in `token_refresh_failure.go` includes a `provider_error` attribute
// with the provider's error string; we pin that it stays a bounded RFC
// 6749 code, not a dump of the response body that includes the access /
// refresh token.
func TestRedaction_RefreshFailureLeaksNoSecrets(t *testing.T) {
	rig := newProxyRefreshRig(t, "redaction-refresh-fail")
	connID := rig.completeAuthFlow(t)

	token := rig.env.GetOAuth2Token(t, connID)
	require.NotNil(t, token)
	accessPlaintext := rig.env.DecryptOAuth2AccessToken(t, token)
	refreshPlaintext := rig.env.DecryptOAuth2RefreshToken(t, token)

	rig.forceTokenExpired(t, connID, false)
	rig.provider.Script(rig.clientKey, helpers.EndpointRefresh, helpers.ScriptAction{
		Status: 400,
		Body:   rfc6749Error("invalid_grant"),
	})

	w := rig.env.DoProxyRequest(t, connID, rig.provider.ResourceURL("/echo"), "GET")
	require.NotEqualf(t, 200, w.Code, "proxy must NOT return 200 when refresh fails with invalid_grant; got 200 body=%s", w.Body.String())
	// Sanity: failure event present in the capture.
	require.NotEmpty(t, rig.logCapture.RecordsWithMessage(t, tokenRefreshFailureMessage),
		"refresh-failed event must be emitted on invalid_grant")

	secrets := flowSecrets{
		ClientSecret:      extractClientSecret(t, rig.clientKey),
		AccessTokenValue:  accessPlaintext,
		RefreshTokenValue: refreshPlaintext,
	}

	assertConnectionIDPresent(t, rig.logCapture, connID)
	assertNoSecretsInLogs(t, rig.logCapture, secrets)
}

// extractClientSecret recovers the client_secret value the rig configured
// the connector with. The rigs use the deterministic suffix pattern
// `<name>-secret-<unix-nano>`, parallel to `<name>-client-<unix-nano>`,
// so we derive the secret from the known clientKey.
func extractClientSecret(t *testing.T, clientKey string) string {
	t.Helper()
	idx := strings.Index(clientKey, "-client-")
	require.Greaterf(t, idx, 0, "expected clientKey shaped <name>-client-<suffix>; got %q", clientKey)
	name := clientKey[:idx]
	suffix := clientKey[idx+len("-client-"):]
	require.NotEmpty(t, suffix, "clientKey missing suffix: %q", clientKey)
	return name + "-secret-" + suffix
}
