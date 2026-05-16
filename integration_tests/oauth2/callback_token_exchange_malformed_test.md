# OAuth2 Malformed Token Responses (scenario 17)

Companion specification for `callback_token_exchange_malformed_test.go`.
Covers issue #176 scenario 17 — the provider returns 200 with a
malformed or incomplete token-response body and the proxy must:

1. Fail safely (no partial credentials persisted).
2. Not overwrite existing valid credentials.
3. Classify the failure as a provider response error
   (`malformed_response`).

Scope is the **token endpoint** at the authorization-code exchange leg.
The same shape of bug applies to the refresh leg (which reuses
`createDbTokenFromResponse`); refresh-side coverage lives in
`proxy_refresh_failure_test.go` and is cross-referenced below.

## The twelve sub-cases

Issue #176 lists twelve malformed shapes. Three are currently rejected
by the proxy; nine are currently accepted. Both groups are tested in
this file — the accepted group as a regression guard so that a future
shift in validation (in either direction) is observable.

### Rejected as `malformed_response` (3)

`TestTokenExchangeMalformed_FailsSafely`:

| Sub-case | Body | Why it fails today |
| --- | --- | --- |
| `InvalidJSON` | `{not valid json` | `json.Decoder.Decode` returns a parser error before any field is inspected. |
| `MissingAccessToken` | `{"token_type":"Bearer","expires_in":3600,"refresh_token":"r"}` | Explicit empty-string check in `createDbTokenFromResponse` (`token_response.go:39-41`). |
| `NonIntegerExpiresIn` | `{"access_token":"X","token_type":"Bearer","expires_in":"3600"}` | `tokenResponse.ExpiresIn` is `*int`; a JSON string for that field fails type-checked unmarshal. Surfaces as the same `malformed_response` category as the parser-error path. |

Each row asserts:

- one `oauth token exchange failed` event with `category=malformed_response`,
- no token row persisted (`GetOAuth2Token` returns nil),
- connection state stays `created`, `setup_step=auth_failed`,
- exactly one `/token` POST observed.

### Currently accepted (9)

`TestTokenExchangeMalformed_CurrentlyAccepted`:

| Sub-case | Mutation | Why the proxy accepts it today |
| --- | --- | --- |
| `WrongContentType` | JSON body declared `Content-Type: text/html` | gentleman's `Response.JSON` decodes the reader directly and ignores `Content-Type`; the body parses regardless of the declared media type. |
| `MissingTokenType` | omit `token_type` | `tokenResponse.TokenType` is a plain string; absence decodes to `""` and the proxy never checks it. |
| `UnsupportedTokenType` | `"token_type":"MAC"` | `tokenResponse.TokenType` is accepted verbatim; only Bearer is supported at the resource layer, so an unsupported type silently surfaces later when the proxied request is sent. |
| `MissingExpiresIn` | omit `expires_in` | `tokenResponse.ExpiresIn` is `*int`; nil decodes to no `expires_at`, which the proxy treats as never-expiring. |
| `NegativeExpiresIn` | `"expires_in":-3600` | Negative values are unvalidated; `expires_at` lands in the past, so the next proxy call refreshes immediately. |
| `ShortExpiry` | `"expires_in":1` | Identical code path to a normal positive expiry; no minimum-validity sanity floor. |
| `LongExpiry` | `"expires_in":1576800000` (50 years) | Identical code path; no maximum-validity sanity cap. Value chosen to stay well within `time.Duration` range when multiplied by `time.Second`. |
| `DuplicateFields` | `{"access_token":"first","access_token":"X",…}` | Go's `encoding/json` silently accepts duplicate object keys (last-write-wins). |
| `MissingScope` | omit `scope` | RFC 6749 §5.1 explicitly permits omitting `scope` when granted == requested; the proxy's fallback to the requested scope is spec-correct. **Not a gap** — included as a positive control so the full 12-case surface is testable in one place. |

Each row asserts:

- no `oauth token exchange failed` event,
- connection advances to `state=Ready`, `setup_step=nil`, `setup_error=nil`,
- a token row is persisted,
- exactly one `/token` POST observed.

For the spec-violating rows (every row except `MissingScope`), the
assertion `GetOAuth2Token != nil` is the **regression guard**: when
validation is added later, that assertion fails and the case moves into
`TestTokenExchangeMalformed_FailsSafely`. Each row carries a `note`
explaining why the proxy accepts it today so a reviewer of a future
validation PR knows what the gap was.

## Why a separate "currently accepted" test instead of follow-up bugs

Splitting the gap cases off into separate follow-up issues without a
test would leave the failure mode silent — the proxy's behaviour would
remain undocumented until someone exercises one of the shapes in a
production incident. Documenting the gaps as passing tests captures the
observed behaviour at the time of writing, makes the gap visible in the
test suite, and turns "we now validate `token_type`" into a single
diff-able assertion update.

When the proxy's validation surface is expanded, the workflow is:

1. Pick a row from `TestTokenExchangeMalformed_CurrentlyAccepted`.
2. Add the validation.
3. Re-run the suite — the row fails on the `GetOAuth2Token != nil`
   assertion.
4. Move the row into the rejected table, updating the assertion to
   `malformed_response`.

This is the same pattern `proxy_refresh_failure_test.go` uses for
`NoAccessTokenInResponse` (folded into `malformed_response` despite the
RFC arguably wanting a distinct category) — see issue #189 for the
related discussion.

## Property 2: "existing valid credentials are not overwritten"

Issue #176 lists this as a separate verify item. The exchange path
runs at initial connect — there is no prior token row to overwrite, so
the property is vacuously satisfied here. The load-bearing version of
this property lives on the **refresh** path, where a prior token row
exists when the failing refresh occurs.

That refresh-side property is already covered by
`proxy_refresh_failure_test.go`'s `MalformedResponse` and
`NoAccessTokenInResponse` rows (`refreshFailureCases`):

- snapshot the token row id and `EncryptedRefreshToken` before the
  refresh,
- drive the failing refresh,
- assert post-failure: same id, same encrypted refresh-token bytes
  (`proxy_refresh_failure_test.go:199-203`).

That assertion holds because `createDbTokenFromResponse` returns the
parse error *before* `InsertOAuth2Token` is reached — no replacement
row is created, so `GetOAuth2Token` continues returning the original
row. Reflecting it explicitly across both paths would be duplicate
coverage; the cross-reference here is the documentation.

## What is *not* covered here

- **Refresh-leg malformed responses.** Covered by
  `proxy_refresh_failure_test.go` (cases `MalformedResponse` and
  `NoAccessTokenInResponse`). Both legs share
  `createDbTokenFromResponse`, so a regression in the parser would
  surface in either suite.
- **Bytes-on-the-wire integrity.** This test scripts the response
  body; it does not exercise the connection-drop / partial-write
  surface. Transport-level failure modes are scenario 18's territory
  (`#177`).
- **Provider returning 4xx with malformed JSON body.** Already covered
  by `TestTokenExchangeRejection_Provider4xxOther` — when the status
  is 4xx the body shape is irrelevant to classification, the category
  is the status-bucket.
- **Time-overflow expires_in.** Issue #176 calls out "extremely long
  expiry" but does not specify a numeric extreme. `LongExpiry` uses 50
  years (well within `time.Duration` range); the int64-overflow case
  would produce non-deterministic wraparound and is left for a
  dedicated test if and when the proxy adds a sanity cap.

## Components

| Lever | What it controls |
| --- | --- |
| `tokenExchangeFailureRig` + `initiateAndMintCode` + `scriptTokenEndpoint` | Per-test fixture shared with `callback_token_exchange_failure_test.go`. Each subtest spins its own rig so the per-fixture `LogCapture` + script queue don't bleed across rows. |
| `helpers.ScriptAction{Status:200, Body, Headers}` | Per-row response shape. `Body` is the verbatim payload (we don't use `BodyTemplate` for these — each shape is bespoke). `Headers` is used by the `WrongContentType` row. |
| `tokenExchangeFailureMessage` | The "oauth token exchange failed" message string the proxy emits on every failure (`internal/auth_methods/oauth2/token_exchange_failure.go`). Used to count failure events. |
| `malformedExchangeRejectCases` | Three-row table of currently-rejected sub-cases. |
| `malformedExchangeAcceptCases` | Nine-row table of currently-accepted sub-cases (with notes). |
