# OAuth2 Malformed Callbacks

Companion specification for `callback_malformed_test.go`. The callback
handler must reject callback URLs whose query-param shape violates RFC
6749 §4.1.2: an authorization response should contain either `code`
or `error`, not both, and a successful response must include `code`.

## Scope

Two tests, one per malformed shape:

1. **`TestMalformedCallback_CodeAndErrorRejectedNoExchange`** —
   callback URL carries both `code=...` AND `error=...`. The proxy
   must classify the failure as `provider_denied` and refuse to
   exchange the code at the token endpoint.
2. **`TestMalformedCallback_MissingCodeNoError`** — callback URL
   carries valid `state` but neither `code` nor `error`. The proxy
   must classify the failure as `missing_code` and reject the flow
   without contacting the token endpoint.

Both tests reuse `tokenExchangeFailureRig` (from
`callback_token_exchange_failure_test.go`); the post-failure
observable surface is identical to the rest of the token-exchange
rejection suite.

## Why one shared file

The two scenarios share the *callback-shape rejection* code path —
both fire from the early-return guards in `callback.go:117-134`,
before any token-endpoint logic runs. Splitting them across two
files would duplicate the rig and the "no /token call observed"
assertion, and would obscure the shared property that the load-bearing
guard for both is "never reach the token endpoint when the callback
is malformed."

A single companion file also makes the inverse property obvious to a
reviewer: every other test in the directory drives the token endpoint
via the rig; this file is the one place where the token endpoint must
*not* be called.

## What each test pins

### `TestMalformedCallback_CodeAndErrorRejectedNoExchange`

- Callback URL: `?state=<valid>&code=<minted>&error=access_denied&error_description=user+denied`
- Exactly one `oauth token exchange failed` event with:
  - `category = provider_denied`
  - `provider_error = access_denied`
  - `state_id` populated
  - `provider_status_code` field **absent** — the proxy never POSTed
    to `/token`, so there is no HTTP status to attach.
- Connection lands in `state=created`, `setup_step=auth_failed`,
  `setup_error` populated (contains `access_denied`).
- No token row persisted.
- 302 redirect to `return_to_url?setup=pending&connection_id=<id>` so
  the marketplace UI re-renders the connection in its failed state.
- **Zero POSTs to `/token`** — the load-bearing assertion. A single
  call here would mean the proxy missed the `error=` signal and
  burned the authorization code against the provider for an
  authorization the server already said it would not honour.

The code parameter is minted via `initiateAndMintCode` (the same path
every other test in the file uses) and then attached to a malformed
callback URL. Minting a real code rather than a synthetic string is
deliberate: it proves the proxy refused to exchange a *valid* code
purely because of the `error=` signal accompanying it, not because the
code was structurally unparseable.

### `TestMalformedCallback_MissingCodeNoError`

- Callback URL: `?state=<valid>` (no `code`, no `error`)
- Exactly one `oauth token exchange failed` event with:
  - `category = missing_code`
  - `state_id` populated
  - `provider_status_code` **absent** — no token-endpoint POST happened.
  - `provider_error` **absent** — no `error=` was in the callback.
- Connection lands in `state=created`, `setup_step=auth_failed`,
  `setup_error` populated (contains `no code in query`).
- No token row persisted.
- 302 redirect to `return_to_url?setup=pending&connection_id=<id>`.
- **Zero POSTs to `/token`** — there is no material for the POST
  (the only thing it could POST is the code, and there isn't one).

`initiateAndMintCode` is still used here for fixture consistency with
the rest of the suite — the rig wires up the state row, connection,
and provider scripting context the same way for every test. The
minted code is discarded; the callback URL deliberately omits it.

## Why `provider_denied` for the code+error case (not a separate category)

RFC 6749 §4.1.2.1 defines `error=...` as the denial-shaped redirect.
The §4.1.2.1 spec text says the authorization server "MUST NOT" issue
an authorization code in a denial redirect, so an `error` query
*always* implies the code (if present) is not a usable grant — same
operator interpretation as the no-code-just-error case. Folding both
shapes into `provider_denied` keeps dashboards simple: one category
for "the provider's redirect carried a denial signal." Distinguishing
"clean denial" from "denial with stale code" would split alerting on
a provider-side bug the operator cannot fix.

If we ever want to surface the "provider sent a stale code with the
denial" anomaly as its own signal, the right place to add it is a
secondary attribute on the existing `provider_denied` event (e.g.,
`code_present=true`) rather than a new category. That keeps the
primary alerting axis stable.

## Property: "code is not exchanged"

Malformed callbacks must not exchange an authorization code unless the
callback shape is explicitly allowed. Operationally that property is
visible as a single observable: **zero POSTs to the provider's
`/token` endpoint** when the callback is malformed. Both tests pin this
directly with `provider.Requests(RequestsFilter{Endpoint:
EndpointToken, ClientID: rig.clientKey})`.

The filter uses `ClientID = rig.clientKey` rather than `Since =
time.Now()` because each rig provisions a fresh client key on the
shared docker provider; only this rig's flow can produce a /token
call tagged with this key. Other tests running concurrently against
the same provider container do not appear in the filter result.

## What is *not* covered here

- **Token endpoint refusing the code.** If the proxy did exchange the
  code despite the malformed callback shape, the provider would
  likely respond `invalid_grant` (RFC 6749 §5.2). That is the
  `TestTokenExchangeRejection_InvalidGrant` case in
  `callback_token_exchange_failure_test.go`. This file's "zero
  /token calls" assertion makes the inverse property — the proxy
  must not reach that path — a separate failure mode.
- **Replayed / cross-tenant / unknown state.** Those callback-state
  shapes are covered by `callback_state_security_test.go`,
  `callback_actor_mismatch_test.go`, and
  `callback_cross_namespace_test.go`. This file is specifically about
  the `code`/`error` query-param shape with an otherwise-valid state
  row.
- **PKCE on the callback.** PKCE callback behavior is covered by
  `pkce_test.go`, which verifies missing and mismatched
  `code_verifier` shapes through the provider's real PKCE validator.
- **HTML / non-redirect responses from the authorize endpoint.** Not
  reachable through the proxy — the user's browser interacts with the
  authorize endpoint directly. A hung authorize page is also out of
  scope (`provider_timeouts_test.md` notes the same exclusion).

## Cross-references

| Property | Where else covered |
| --- | --- |
| User denial via `error=access_denied` (callback carries error, no code) | `user_denial_test.go` — end-to-end through the browser UI. Same category (`provider_denied`), but the trigger is a real user click rather than a forged callback. |
| Token-endpoint rejection of an exchanged code | `callback_token_exchange_failure_test.go` — covers `invalid_grant`, `invalid_client`, and the rest of RFC 6749 §5.2. Those tests exercise the path *after* the callback shape is accepted; this file exercises rejection *before* the token endpoint is reached. |
| Malformed *response* from the token endpoint | `callback_token_exchange_malformed_test.go`. Distinct from this file: that one tests broken bodies in the provider's response to a successful POST. This file tests malformed callbacks *to* the proxy, before any POST happens. |
| Callback state security (CSRF, replay, cross-tenant) | `callback_state_security_test.go` and friends. Orthogonal — those tests vary the `state` row; this file varies the `code`/`error` query params with a valid state. |

## Components

| Lever | What it controls |
| --- | --- |
| `tokenExchangeFailureRig` + `initiateAndMintCode` + `requireOneFailureEvent` + `requireAuthFailedConnection` + `requireRedirectToReturnURL` | Per-test fixture and shared assertions; reused from `callback_token_exchange_failure_test.go`. |
| `env.ForgeOAuth2CallbackURL(state, "")` | Builds the missing-code callback URL. Existing helper; passing `code=""` already produces `?state=<id>` with no code parameter. |
| `forgeCallbackURLWithErrorAndCode(env, state, code, errCode, errDescription)` | New helper local to this file — covers the code-plus-error shape that `ForgeOAuth2CallbackURL` can't emit. |
| `env.DeliverOAuth2Callback(t, url)` | Drives the in-process callback delivery; reused unchanged. |
| `provider.Requests(RequestsFilter{Endpoint: EndpointToken, ClientID: rig.clientKey})` | Counts `/token` POSTs scoped to this rig's client key. The zero-length assertion proves malformed callbacks are rejected before token exchange. |
| `tokenExchangeProviderDenied = "provider_denied"`, `tokenExchangeMissingCode = "missing_code"` | Category strings pinned by the tests. Defined in `internal/auth_methods/oauth2/token_exchange_failure.go:22, 25`. |
