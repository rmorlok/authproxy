//go:build integration

package oauth2

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
)

// Scenarios 15 and 16 from issue #175: the callback handler must reject
// callback URLs whose query-param shape violates RFC 6749 §4.1.2 (the
// `code` xor `error` contract). Both shapes share the same observable
// rejection signature — auth_failed connection, single failure log
// event, no token persisted, redirect to return_to_url — but pin
// different category strings and exercise different branches of
// callback.go:117-134.
//
// The two cases:
//
//   - **Scenario 15: code + error.** Per RFC 6749 §4.1.2.1 a denial
//     redirect carries `error=...` *instead of* `code=...`. The
//     authorization server SHOULD NOT include both. If the proxy
//     receives both, the safe interpretation is to treat the response
//     as a failure and refuse to exchange the code — exchanging a code
//     attached to a denial signal would burn the code against the
//     provider for an authorization the server already said it would
//     not honour. The proxy's current implementation
//     (`callback.go:117-127`) reads `error` first and returns before
//     looking at `code`, so the code is structurally unreachable in
//     this shape. Tests pin **zero** /token calls to lock this in.
//
//   - **Scenario 16: missing code, no error.** A callback with valid
//     state but neither `code` nor `error` is malformed — there is no
//     credential to exchange and no rejection signal to surface. The
//     proxy emits `missing_code` (`callback.go:129-134`) and never
//     reaches the token endpoint.
//
// Both cases reuse `tokenExchangeFailureRig` because the post-failure
// observables (auth_failed setup step, failure log, return_to_url
// redirect) are identical to the rest of the token-exchange rejection
// suite — only the trigger differs. See
// `callback_token_exchange_failure_test.go` for the canonical example.

// forgeCallbackURLWithErrorAndCode builds `/oauth2/callback?...` with
// `state`, `code`, and `error` query params. `env.ForgeOAuth2CallbackURL`
// can only emit state and code; this helper covers the code-plus-error
// shape that lives only in this test file.
func forgeCallbackURLWithErrorAndCode(env *helpers.IntegrationTestEnv, state, code, errCode, errDescription string) string {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if code != "" {
		q.Set("code", code)
	}
	if errCode != "" {
		q.Set("error", errCode)
	}
	if errDescription != "" {
		q.Set("error_description", errDescription)
	}
	base := env.PublicOAuthCallbackURL()
	if encoded := q.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

// TestMalformedCallback_CodeAndErrorRejectedNoExchange — provider
// redirects with both `code=...&error=access_denied&...`. The proxy
// must treat this as a denial-shaped failure and refuse to POST the
// code to the token endpoint.
//
// The choice of `error=access_denied` mirrors the most plausible
// real-world source of this shape: a provider that always echoes the
// originally-issued code into the redirect, even when the user
// declines on the consent screen. The exact `error` value doesn't
// change the categorization — any non-empty `error` produces
// `provider_denied`.
func TestMalformedCallback_CodeAndErrorRejectedNoExchange(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "mc-code-and-error")
	connID, stateID, code := rig.initiateAndMintCode(t)
	require.NotEmpty(t, code, "fixture sanity: provider must mint a code so we can prove it isn't exchanged")

	callbackURL := forgeCallbackURLWithErrorAndCode(rig.env, stateID, code, "access_denied", "user denied")
	loc := rig.env.DeliverOAuth2Callback(t, callbackURL)
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "provider_denied")
	assert.Equal(t, stateID, event["state_id"], "failure event should carry the state_id for correlation")
	assert.Equal(t, "access_denied", event["provider_error"],
		"failure event should record the provider's error code verbatim")
	_, hasStatus := event["provider_status_code"]
	assert.Falsef(t, hasStatus,
		"provider_status_code must be absent: the proxy never POSTed to /token, there is no HTTP status to record (got %v)",
		event["provider_status_code"])

	require.Nil(t, rig.env.GetOAuth2Token(t, connID),
		"no token row should be persisted when the callback carries an error signal")
	rig.requireAuthFailedConnection(t, connID, "access_denied")

	// The load-bearing assertion: the proxy MUST NOT POST the code to
	// the token endpoint when the callback also carries an error. This
	// is what makes the "code is not exchanged" property in issue #175
	// observable — a single /token call here would mean the proxy
	// missed the error signal and burned the code against the provider.
	tokenReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: rig.clientKey,
	})
	assert.Lenf(t, tokenReqs, 0,
		"proxy must not POST to /token when callback carries an error signal; got %d call(s)", len(tokenReqs))
}

// TestMalformedCallback_MissingCodeNoError — callback URL carries a
// valid state but has neither `code` nor `error`. The proxy classifies
// this as `missing_code` (callback.go:129-134) and rejects the flow
// without contacting the token endpoint.
//
// We still drive the provider's authorize step via
// `initiateAndMintCode` so the rig's fixture work (state row written
// to Redis, connection row created in `created` state) matches every
// other failure test in the suite. The minted code is discarded — the
// purpose of this test is the *absence* of a code in the callback URL.
func TestMalformedCallback_MissingCodeNoError(t *testing.T) {
	rig := newTokenExchangeFailureRig(t, "mc-missing-code")
	connID, stateID, _ := rig.initiateAndMintCode(t)

	// ForgeOAuth2CallbackURL with code="" produces `?state=<id>` — no
	// code, no error. Exactly the malformed shape scenario 16 names.
	callbackURL := rig.env.ForgeOAuth2CallbackURL(stateID, "")
	loc := rig.env.DeliverOAuth2Callback(t, callbackURL)
	rig.requireRedirectToReturnURL(t, connID, loc)

	event := rig.requireOneFailureEvent(t, "missing_code")
	assert.Equal(t, stateID, event["state_id"])
	_, hasStatus := event["provider_status_code"]
	assert.Falsef(t, hasStatus,
		"provider_status_code must be absent: there is no token-endpoint POST in this code path (got %v)",
		event["provider_status_code"])
	_, hasProviderError := event["provider_error"]
	assert.Falsef(t, hasProviderError,
		"provider_error must be absent: no `error` query param was supplied (got %v)",
		event["provider_error"])

	require.Nil(t, rig.env.GetOAuth2Token(t, connID),
		"no token row should be persisted when the callback is missing code")
	rig.requireAuthFailedConnection(t, connID, "no code in query")

	// Same load-bearing property as the code+error case: missing-code
	// must not produce a token-endpoint POST. The proxy's only
	// material for the POST is the code, and there is none.
	tokenReqs := rig.provider.Requests(helpers.RequestsFilter{
		Endpoint: helpers.EndpointToken,
		ClientID: rig.clientKey,
	})
	assert.Lenf(t, tokenReqs, 0,
		"proxy must not POST to /token when callback is missing code; got %d call(s)", len(tokenReqs))
}
