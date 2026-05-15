//go:build integration

package oauth2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/database"
)

// Scenario 17 from issue #176: the provider's /token endpoint returns a
// 200 response whose shape is malformed in one of twelve ways. The
// per-spec expectation is that every malformed shape "fails safely" —
// no token persisted, classified as a provider response error, existing
// valid credentials untouched.
//
// In the proxy's current implementation, only three of the twelve
// sub-cases are actually rejected (`tokenExchangeMalformedResponse`):
//
//   - Invalid JSON — `tokenResponse.Decode` returns a parser error.
//   - Missing `access_token` — the explicit empty-string check in
//     `createDbTokenFromResponse`.
//   - Non-integer `expires_in` — `tokenResponse.ExpiresIn` is a `*int`,
//     so a string-shaped value fails JSON unmarshalling.
//
// The remaining nine sub-cases (wrong content type, missing/unsupported
// `token_type`, missing/negative/short/long `expires_in`, duplicate
// fields, missing `scope`) are accepted by the proxy today: it persists
// a token row and advances the connection to Ready. The
// `TestTokenExchangeMalformed_CurrentlyAccepted` table documents this so
// regressions in either direction are observable:
//
//   - if validation is added later, the assertion that a token persisted
//     will fail and the case moves into the "FailsSafely" table;
//   - if the proxy ever starts rejecting an additional shape silently,
//     the failure event count diverges from the documented expectation.
//
// The spec says "Existing valid credentials are not overwritten" is the
// load-bearing second property. The exchange path runs only at initial
// connect, when there is no prior token row to overwrite; the same
// property on the refresh path (where a prior row exists) is covered by
// `proxy_refresh_failure_test.go`'s `MalformedResponse` /
// `NoAccessTokenInResponse` cases, which snapshot the row before the
// failure and assert id + encrypted_refresh_token are unchanged after.
// See the .md companion for the cross-reference.

// malformedExchangeAccessToken is the access_token value used in the
// "accepted" cases. Unique enough that a future regression test could
// re-decrypt the persisted row and confirm it.
const malformedExchangeAccessToken = "malformed-test-at-00000000-0000-4000-8000-000000000001"

// validNoScopeBody is the well-formed token response body used by the
// "missing scope" test. RFC 6749 §5.1 permits omitting `scope` when the
// granted scope is identical to the requested scope, so this is not a
// gap — the proxy's fallback to the requested scope is spec-correct.
const validNoScopeBody = `{"access_token":"` + malformedExchangeAccessToken +
	`","token_type":"Bearer","expires_in":3600}`

type malformedExchangeRejectCase struct {
	name string
	// body sent on the /token response. Status is always 200 for the
	// scenario-17 surface — the spec is about 200 + bad shape, not 4xx
	// (4xx is covered by callback_token_exchange_failure_test.go).
	body string
}

// rejectCases enumerates the malformed shapes the proxy currently
// rejects as `malformed_response`. Distinct rows so the assertion
// failure message names the exact sub-case when something regresses.
var malformedExchangeRejectCases = []malformedExchangeRejectCase{
	{
		// RFC 6749 §5.1: the token response is JSON. A body the JSON
		// decoder cannot parse fails before any field check.
		name: "InvalidJSON",
		body: `{not valid json`,
	},
	{
		// Well-formed JSON, but `access_token` is missing —
		// `createDbTokenFromResponse` rejects this explicitly because
		// there is no credential to persist.
		name: "MissingAccessToken",
		body: `{"token_type":"Bearer","expires_in":3600,"refresh_token":"r"}`,
	},
	{
		// `expires_in` declared as a JSON string instead of an integer.
		// The `*int` field rejects the type mismatch at unmarshal time,
		// so the request never reaches the empty-token check — it
		// surfaces as the same malformed_response category by way of
		// the parser failure.
		name: "NonIntegerExpiresIn",
		body: `{"access_token":"X","token_type":"Bearer","expires_in":"3600"}`,
	},
}

// TestTokenExchangeMalformed_FailsSafely — every case in
// malformedExchangeRejectCases asserts the spec property "invalid
// responses fail safely": malformed_response category, no token row
// persisted, connection in auth_failed terminal state.
func TestTokenExchangeMalformed_FailsSafely(t *testing.T) {
	for _, tc := range malformedExchangeRejectCases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newTokenExchangeFailureRig(t,
				fmt.Sprintf("te-mal-rej-%s", strings.ToLower(tc.name)))
			connID, stateID, code := rig.initiateAndMintCode(t)

			rig.scriptTokenEndpoint(helpers.ScriptAction{
				Status: 200,
				Body:   tc.body,
			})

			loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
			rig.requireRedirectToReturnURL(t, connID, loc)

			rig.requireOneFailureEvent(t, "malformed_response")
			require.Nilf(t, rig.env.GetOAuth2Token(t, connID),
				"%s: no token row should persist on malformed response", tc.name)
			rig.requireAuthFailedConnection(t, connID, "")
			rig.requireOneTokenCallObserved(t)
		})
	}
}

type malformedExchangeAcceptCase struct {
	name string
	// body sent on the /token response. All bodies are well-formed JSON
	// with a non-empty access_token; the malformed aspect is in a
	// neighbouring field (token_type / expires_in / etc.) or in the
	// response headers (wrong content type / duplicates).
	body    string
	headers map[string]string
	// note documents why the proxy currently accepts the case and what
	// would have to change to reject it. The matching .md companion has
	// the full classification + follow-up gap list.
	note string
}

// validTokenBody is the well-formed token response we mutate to produce
// the accepted-malformed sub-cases below. Each case alters exactly one
// dimension so the surviving assertion (token persists, Ready, no
// failure event) is attributable to that dimension.
const validTokenBodyTemplate = `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"Bearer","expires_in":3600,"scope":"read"}`

// malformedExchangeAcceptCases enumerates the nine scenario-17 sub-cases
// the proxy currently accepts. Each persists a token row and advances
// the connection to Ready. The .md companion tracks follow-up issues
// for the spec violations.
var malformedExchangeAcceptCases = []malformedExchangeAcceptCase{
	{
		// RFC 6749 §5.1 mandates `application/json;charset=UTF-8`. The
		// proxy uses encoding/json directly against the response
		// reader, which ignores Content-Type — so a JSON-shaped body
		// declared as text/html still parses.
		name:    "WrongContentType",
		body:    validTokenBodyTemplate,
		headers: map[string]string{"Content-Type": "text/html"},
		note:    "gentleman + encoding/json ignore Content-Type; the body parses regardless of the declared media type.",
	},
	{
		// `tokenResponse` does not require `token_type`; a missing
		// field unmarshals to the zero value and the proxy never
		// checks it.
		name: "MissingTokenType",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","expires_in":3600,"scope":"read"}`,
		note: "tokenResponse.TokenType is unvalidated; absence is accepted and the proxy proceeds with Bearer semantics by default.",
	},
	{
		// Similarly, the proxy does not reject a non-Bearer
		// token_type. RFC 6749 §7.1 anticipates extension types, but
		// the proxy itself only supports Bearer at the resource layer
		// — accepting `MAC` here would lead to a downstream failure
		// when the proxied request is sent.
		name: "UnsupportedTokenType",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"MAC","expires_in":3600,"scope":"read"}`,
		note: "tokenResponse.TokenType is accepted verbatim; non-Bearer values are not rejected at exchange time.",
	},
	{
		// `tokenResponse.ExpiresIn` is `*int`; a missing field
		// unmarshals to nil and the persisted row gets no expires_at.
		// The proxy then treats the access token as non-expiring at
		// the local-clock layer.
		name: "MissingExpiresIn",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"Bearer","scope":"read"}`,
		note: "tokenResponse.ExpiresIn is *int; nil decodes to no expires_at, which the proxy treats as never-expiring.",
	},
	{
		// Negative expires_in is unvalidated; the persisted token
		// gets an expires_at in the past, and the very next proxied
		// request will trigger a refresh.
		name: "NegativeExpiresIn",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"Bearer","expires_in":-3600,"scope":"read"}`,
		note: "Negative expires_in is accepted; expires_at lands in the past and the next proxy call refreshes immediately.",
	},
	{
		// expires_in=1: same code path as a normal positive expiry.
		// We do not assert that the token is *immediately* usable for
		// a proxied call (timing-dependent); only that the exchange
		// completes.
		name: "ShortExpiry",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"Bearer","expires_in":1,"scope":"read"}`,
		note: "expires_in=1 is accepted as-is; the proxy has no minimum-validity sanity floor.",
	},
	{
		// 50 years in seconds: chosen to stay safely within
		// time.Duration range when multiplied by time.Second
		// (1.58e9 * 1e9 = 1.58e18 ns < int64 max ~9.22e18).
		// time.Duration overflows are a related concern but not the
		// shape this row exercises.
		name: "LongExpiry",
		body: `{"access_token":"` + malformedExchangeAccessToken + `","token_type":"Bearer","expires_in":1576800000,"scope":"read"}`,
		note: "Multi-decade expires_in is accepted; the proxy has no maximum-validity sanity cap.",
	},
	{
		// `{"access_token":"A","access_token":"B",...}` — Go's
		// encoding/json silently takes the last occurrence, so the
		// persisted access_token is "B". The shape is spec-illegal
		// (JSON forbids duplicate object members in strict
		// interpretation) but the parser accepts it.
		name: "DuplicateFields",
		body: `{"access_token":"first-value","access_token":"` + malformedExchangeAccessToken +
			`","token_type":"Bearer","expires_in":3600,"scope":"read"}`,
		note: "encoding/json accepts duplicate keys (last-write-wins); the proxy does not reject the response.",
	},
	{
		// Spec-permissive case, NOT a gap. RFC 6749 §5.1 allows
		// omitting `scope` when the granted scope is identical to the
		// requested scope. The proxy's fallback to the requested
		// scope is spec-correct; included here so the full 12-case
		// surface is covered in one place.
		name: "MissingScope",
		body: validNoScopeBody,
		note: "RFC 6749 §5.1 permits omitting scope when granted == requested; proxy intentionally falls back to requested.",
	},
}

// TestTokenExchangeMalformed_CurrentlyAccepted — every case in
// malformedExchangeAcceptCases asserts the proxy persists a token row
// and advances the connection to Ready, with no token-exchange failure
// event emitted. For the spec-violating rows, the assertion is the
// regression guard: if the proxy ever starts validating the dimension,
// the test will fail and the case moves to the "FailsSafely" table.
// MissingScope is included as a positive-control row — see its `note`.
func TestTokenExchangeMalformed_CurrentlyAccepted(t *testing.T) {
	for _, tc := range malformedExchangeAcceptCases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newTokenExchangeFailureRig(t,
				fmt.Sprintf("te-mal-ok-%s", strings.ToLower(tc.name)))
			connID, stateID, code := rig.initiateAndMintCode(t)

			rig.scriptTokenEndpoint(helpers.ScriptAction{
				Status:  200,
				Body:    tc.body,
				Headers: tc.headers,
			})

			loc := rig.env.DeliverOAuth2Callback(t, rig.env.ForgeOAuth2CallbackURL(stateID, code))
			require.Truef(t, strings.HasPrefix(loc, rig.returnToURL),
				"%s (%s): accepted exchange should land on return_to_url; got %q", tc.name, tc.note, loc)

			// No failure event — the spec violation is silent.
			assert.Emptyf(t, rig.logCapture.RecordsWithMessage(t, tokenExchangeFailureMessage),
				"%s (%s): accepted shape must not emit a token-exchange-failed event", tc.name, tc.note)

			// Connection advanced to Ready, no auth_failed setup_step.
			conn := rig.env.GetConnection(t, connID)
			assert.Equalf(t, database.ConnectionStateReady, conn.State,
				"%s (%s): accepted exchange should transition to Ready", tc.name, tc.note)
			assert.Nilf(t, conn.SetupStep,
				"%s (%s): accepted exchange must not record an auth_failed setup_step", tc.name, tc.note)
			assert.Nilf(t, conn.SetupError,
				"%s (%s): accepted exchange must not record a setup_error", tc.name, tc.note)

			// Token row was persisted — the proxy treats this as a
			// successful exchange.
			require.NotNilf(t, rig.env.GetOAuth2Token(t, connID),
				"%s (%s): accepted exchange must persist a token row", tc.name, tc.note)

			rig.requireOneTokenCallObserved(t)
		})
	}
}
