# PKCE Validation

## What this verifies

The OAuth2 PKCE machinery wires three pieces together:

1. The authorize URL builder (`internal/auth_methods/oauth2/auth_url.go:96-113`)
   emits `code_challenge` and `code_challenge_method` when the connector
   declares a PKCE block.
2. The state record (`internal/auth_methods/oauth2/state.go:34-46`) persists
   the per-flow `PKCECodeVerifier` and `PKCEMethod` alongside the encrypted
   state envelope in Redis.
3. The callback handler (`internal/auth_methods/oauth2/callback.go:178-183`)
   forwards `code_verifier` from state on the token-exchange POST — but only
   on the initial code-for-token exchange, never on refresh (RFC 7636 §6).

These tests prove the three pieces are wired correctly end-to-end against
a real upstream provider that enforces PKCE (`RequirePKCE: true` on the
client). They sit on top of the unit tests in
`internal/auth_methods/oauth2/pkce_test.go`, which already cover the
generator, the S256/plain transformation, and the connector schema
validator. The integration tests catch what unit tests cannot: that the
challenge the provider receives at authorize is the one the verifier the
proxy persists hashes to, that the persisted verifier actually rides on
the token exchange POST, and that the proxy fails the connection cleanly
when the provider rejects PKCE.

## Coverage matrix

The PKCE integration coverage maps the important success and failure
cases to concrete tests:

| PKCE case                           | Covered by                                              |
|------------------------------------|---------------------------------------------------------|
| Valid `code_verifier`              | `TestPKCE_HappyPath_S256`                               |
| Missing `code_verifier`            | `TestPKCE_TokenExchangeRejected/MissingVerifier`        |
| Invalid `code_verifier`            | `TestPKCE_TokenExchangeRejected/MismatchedVerifier`     |
| Unsupported `code_challenge_method` | Unit tests in `internal/auth_methods/oauth2/pkce_test.go` (`TestAuthOAuth2_Validate_RejectsUnknownPKCEMethod`) — schema validator rejects at config load, so the bad value never reaches an integration test. Calling it out here so the audit trail is complete. |

| Verification point                   | Covered by                                              |
|--------------------------------------|---------------------------------------------------------|
| Valid PKCE succeeds                  | `TestPKCE_HappyPath_S256`                               |
| Invalid PKCE fails token exchange    | Both `TestPKCE_TokenExchangeRejected` subtests          |
| Failed PKCE does not create a connection | Both `TestPKCE_TokenExchangeRejected` subtests       |

## Test 1: `TestPKCE_HappyPath_S256`

**Goal:** prove the full PKCE roundtrip works against a provider that
enforces PKCE, driven through the marketplace UI exactly as a real user
would experience it.

**Setup:**
- Test provider client registered with `RequirePKCE: true`. The provider
  refuses any authorize that arrives without `code_challenge`, so a green
  flow is a positive signal that the proxy is emitting the challenge.
- Connector with `pkce: {method: S256}`.
- chromedp + marketplace + provider UI for the user-facing
  authorization leg.

**Assertions:**
1. Connection landed in `ready`. Token row persisted with non-empty
   encrypted material and a future expiry. With `RequirePKCE: true` on
   the provider client, a `ready` state is a positive signal: the
   provider verified `base64url(sha256(verifier)) == challenge` on its
   side, so if anything had gone wrong on the proxy side (missing
   challenge on authorize, missing/stale verifier on token exchange) the
   flow would have failed before this point.
2. The provider observed exactly one `/token` POST with grant_type
   `authorization_code` and the `code_verifier` form field present
   (non-empty). The test cannot pin the literal verifier value here
   because the provider's request recorder redacts `code_verifier` to
   `<redacted>` per its sensitive-field policy. The literal-value
   contract — that the verifier the proxy persisted in state is the one
   that arrives at the provider, and that it hashes to the challenge —
   is pinned end to end in Test 2 (see "Pre-callback PKCE assertion"
   below).

**Why chromedp over the in-process driver:** the happy path is the
user-driven authorization leg under test. The `/test/authorize`
shortcut would mint a code without exercising the marketplace redirect,
provider login, consent, and browser callback chain. The happy-path
verification depends on the same code that runs in production redirect
chains, not a hand-rolled callback delivery.

## Test 2: `TestPKCE_TokenExchangeRejected`

**Goal:** prove the proxy fails closed when the provider rejects PKCE at
token exchange. Driven via the in-process callback delivery so the test
can mutate the state record between authorize and callback — the only
way to deterministically produce a mismatched or missing verifier (a
correctly wired proxy will never emit one on its own).

**Setup (shared):**
- Test provider client with `RequirePKCE: true`.
- Connector with `pkce: {method: S256}`.
- `InitiateOAuth2Connection` mints the state and connection.
- `FollowOAuth2Redirect` returns the upstream authorize URL with the
  challenge embedded — proves the proxy emitted PKCE on the redirect.
- `provider.Authorize(decision=approve)` mints a real authorization
  code bound to the original `code_challenge`.
- Before delivering the callback, the test reads the state from Redis,
  mutates the verifier, and writes it back at the same key with the
  same TTL.
- `DeliverOAuth2Callback` runs through the production handler. The
  proxy posts to `/oauth/token` with the mutated verifier; the
  provider compares against the stored challenge and rejects.

**Two subtests:**

- `MissingVerifier` — verifier mutated to empty string. The proxy's
  callback skips the `code_verifier` form field (the production check is
  `if o.state.PKCECodeVerifier != ""`), so the provider sees a
  PKCE-bound code with no verifier presented. Mirrors the "what if the
  proxy lost the verifier" failure mode.
- `MismatchedVerifier` — verifier swapped for a freshly generated
  43-char value. The proxy posts a valid verifier shape, but the hash
  doesn't match the stored challenge. Mirrors the "verifier intercepted
  and replaced by an attacker" failure mode (and any honest proxy bug
  that produces a stale verifier).

**Pre-callback PKCE assertion (both subtests):** before mutating state
the test reads the state record from Redis (plaintext after decrypt) and
asserts `base64url(sha256(state.PKCECodeVerifier)) == challenge` from the
upstream authorize URL. This is the literal-value contract the happy
path can't pin (because the request recorder redacts `code_verifier` on
the wire): the verifier the proxy persists is the one whose hash showed
up at authorize. Any drift between authorize-side challenge derivation
and state persistence would surface here.

**Per-subtest assertions:**
1. Callback redirects to `return_to_url?connection_id=<id>&setup=pending`.
   This matches the token-exchange failure path in `callback_token_exchange_failure_test.go`
   shape: a token-exchange failure during setup is a setup failure, and
   the marketplace UI's reconnect prompt fires on the `setup=pending`
   annotation. It does *not* redirect to `error_pages.internal_error`
   because the connection is recoverable — the user can retry.
2. Exactly one structured `oauth token exchange failed` log event with
   `category` of either `invalid_grant` or `provider_4xx_other`. The
   choice depends on whether the upstream 4xx body included an
   `error=invalid_grant` field. The reference test provider
   (`go-oauth2-server`) returns a 4xx for PKCE failures without a §5.2
   `error` field, so the proxy classifies as `provider_4xx_other`; the
   assertion accepts either to stay robust against the provider
   tightening its error body in a future version. The
   `provider_status_code` field is asserted to be in the 4xx range.
3. Token row was *not* created (`env.GetOAuth2Token` returns nil).
4. Connection's `state` stays `created` and `setup_step` is
   `auth_failed` with a non-empty `setup_error` — same terminal state
   any other token-exchange failure lands in, so the marketplace UI's
   reconnect prompt fires.
5. The provider observed exactly one `/token` POST. For
   `MissingVerifier` the form has no `code_verifier` key; for
   `MismatchedVerifier` the form has `code_verifier` present (redacted
   to `<redacted>` on the wire). The literal "the mutated value, not the
   original, was sent" contract is proved by the provider's PKCE
   rejection itself — if the proxy had sent the original verifier, the
   provider would have accepted it, and all the failure-path
   assertions above would have failed differently.

**Why not just script `invalid_grant`:**
`callback_token_exchange_failure_test.go` already covers "provider
returns invalid_grant → connection lands in auth_failed". Pointing the
PKCE test at a scripted response would duplicate that coverage without
exercising any PKCE-specific code. The state-mutation approach forces
the provider's real PKCE validator to fire, so the test fails if the
provider's PKCE enforcement is ever silently weakened or the proxy
stops persisting/sending the verifier.

## Out of scope

- **PKCE on refresh.** RFC 7636 §6 forbids it, and the production code
  (`task_refresh_oauth_token.go`) has no verifier branch to test against
  — the same code path that runs in non-PKCE refresh runs in PKCE
  refresh. The `proxy_refresh_test.go` suite already pins the no-
  verifier shape of refresh POSTs.
- **PKCE with `plain` method.** Unit tested in
  `TestPKCEChallengeFor_Plain` and
  `TestAuthOAuth2_Validate_AcceptsPlain`. `S256` is the recommended
  method and what production connectors use; an integration test for
  `plain` would only exercise the same wiring this file already covers,
  with a weaker transformation.
- **Per-connector enablement migration.** Tracked separately — this
  file proves the mechanism, not which connectors opt in.
