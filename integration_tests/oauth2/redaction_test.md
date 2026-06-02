# OAuth2 Sensitive-Value Redaction

Companion specification for `redaction_test.go`. Logs, errors, and
metrics emitted by any OAuth flow must
not contain access tokens, refresh tokens, authorization codes, client
secrets, PKCE code verifiers, or raw provider credentials.

Logs *may* include connection_id, tenant id, provider id, correlation
id, error category, retry count, and scope-mismatch metadata — those
are the operator-visible identifiers operators correlate alerts
against.

## What is asserted

Three tests, each driving a different code path so the redaction
property is exercised on:

1. **`TestRedaction_HappyPathFlowLeaksNoSecrets`** — full
   authorization-code flow (initiate → provider authorize → callback →
   token exchange → persist) plus one proxied API call. Covers the
   token-response logging path (success leg of
   `callback.go` / `proxy.go`), the persistence path, and the
   proxy-request path. The decrypted access_token and refresh_token
   from the persisted row, plus the client_secret and authorization
   code, are checked against every captured record.
2. **`TestRedaction_TokenExchangeFailureLeaksNoSecrets`** — scripted
   `invalid_grant` on `/token` during callback. Covers
   `token_exchange_failure.go`'s classify/log path. The client_secret
   and authorization code are checked. No tokens are persisted (failed
   exchange), so access/refresh aren't part of the check.
3. **`TestRedaction_RefreshFailureLeaksNoSecrets`** — scripted
   `invalid_grant` on `/token` during refresh. Covers
   `token_refresh_failure.go`'s classify/log path, which is the most
   likely leak vector because error messages historically tend to
   include the failing request's form values and the provider response
   body. The persisted access_token + refresh_token plaintexts and the
   client_secret are checked.

Each test ends with two assertions:

- **`assertConnectionIDPresent`** — positive control. The flow's
  connection_id must appear in *some* captured record, proving the
  capture is wired up. Without this, "no secret found" trivially
  passes when nothing is captured at all.
- **`assertNoSecretsInLogs`** — for each secret in `flowSecrets`, the
  serialized record stream is searched for the literal plaintext.
  Values under 8 characters are skipped (anti-false-positive).

## How the secrets are recovered

| Secret | Source |
| --- | --- |
| `ClientSecret` | Derived from `rig.clientKey` via `extractClientSecret`. Both rigs use the deterministic shape `<name>-client-<unix-nano>` / `<name>-secret-<unix-nano>`. |
| `AuthorizationCode` | Returned by `provider.Authorize(...).RedirectURL`'s `?code=` query param — the exact bytes the provider would have sent in the callback. |
| `AccessTokenValue` | `env.DecryptOAuth2AccessToken(t, token)` — decrypts the persisted `oauth2_tokens.encrypted_access_token` column. |
| `RefreshTokenValue` | `env.DecryptOAuth2RefreshToken(t, token)` — same, for the refresh column. |
| `UserPassword` | Currently always empty (the rigs don't surface it past `CreateUser`). Slot retained so future tests that do drive a password grant can include it without changing the helper. |

Each secret is a substring search, not a structural one — both
attribute leaks (a `slog.String("token", t.AccessToken)`) and
message-string leaks (an error that formats `%v` over the response
body) are caught. The substring scan compares against re-encoded JSON
of every record, so it sees keys, values, and any nested map content
slog emitted.

## What is *not* covered here

- **PKCE code verifiers.** `pkce_test.go` exercises verifier handling,
  but this redaction suite does not currently capture the verifier
  plaintext. Add a `CodeVerifier` field to `flowSecrets` and a
  per-flow capture point if the verifier becomes observable from the
  integration boundary. Adding new secret material without adding it to
  this test is the silent-leak failure mode this suite exists to catch.
- **Gin HTTP access log.** Gin writes its `[api] ... | 200 | ... |
  POST /...` line directly to stdout, not through the configured slog
  handler, so `LogCapture` does not see it. The access log already
  only contains method/path/status/latency — none of the secret
  values would appear there even if it were captured. If the proxy
  ever moves access logging onto slog, this test starts checking it
  automatically.
- **Structured app metrics request events (full HTTP transcripts → ClickHouse).**
  `internal/app_metrics` applies its own redaction layer (see
  `attribution_codec.go`). The integration-test harness does not wire
  app_metrics up by default, so this boundary test cannot exercise
  it. Unit tests in `internal/app_metrics` cover that layer.
- **OpenTelemetry metric attributes.** `telemetry.go` only uses
  bounded enum labels (`result`, `reason`, `revocation_kind`) plus
  the projected connection-label allowlist; no token values can
  reach metric dimensions structurally. The unit tests in
  `telemetry_test.go` pin this.
- **OpenTelemetry span attributes.** Spans currently set
  `authproxy.connector_id` and `authproxy.oauth2.operation`, both
  bounded. No secret values are added on span attributes.
- **API response bodies returned to the calling client.** If the
  proxy ever embeds a provider error body into the response it
  returns to the customer's app, that's a leak vector this test does
  not check — it scans *logs*, not response payloads. The proxy's
  error redaction at the response boundary is tested separately in
  `internal/auth_methods/oauth2/token_refresh_failure_test.go` /
  `token_exchange_failure_test.go`.

## Why substring scan against decrypted plaintext

The wire-format redaction tests in `request_log` are about checking
specific known keys (`Authorization`, `client_secret`, etc.). This
suite is about catching *any* leak path, including ones that emit the
value under an unexpected key. The strongest test against that class of
bug is: the bytes that the provider actually emitted must not appear
anywhere in the captured log stream. Decrypting the persisted row gives
us the exact plaintext the proxy held in memory and is the only source
for the post-rotation refresh_token value.

A weaker version of this test would scan for keys like `access_token`
or `refresh_token` in record maps. That misses leaks via different
keys (`token`, `auth`, `body`, `response`) or via message-string
interpolation. The byte-level substring scan catches all of them.

## Components

| Lever | What it controls |
| --- | --- |
| `proxyRefreshRig` + `completeAuthFlow` | Standard auth-flow fixture; reused for the happy-path and refresh-failure tests. |
| `tokenExchangeFailureRig` | Token-exchange failure fixture (lets us script a 400 invalid_grant on /token mid-callback). |
| `provider.Authorize(...).RedirectURL` | Source of the exact authorization code the provider would have issued. |
| `env.DecryptOAuth2AccessToken(t, token)` / `env.DecryptOAuth2RefreshToken(t, token)` | Plaintext recovery for the encrypted DB columns — the only way to obtain the post-rotation values for the refresh-failure case. |
| `extractClientSecret(t, clientKey)` | Derives the configured client_secret from the deterministic rig naming convention. |
| `LogCapture.Records(t)` | Returns every captured slog record as a parsed map. We re-encode each one to JSON and concatenate into a single haystack to scan. |
| `flowSecrets` | Per-flow bundle of plaintexts the substring scan must not find. Adding a new secret type (e.g. `CodeVerifier` when PKCE lands) extends the check. |
| `assertConnectionIDPresent` | Positive control that the capture path is live. Without this, the negative-check tests pass trivially when no records are captured. |
| `assertNoSecretsInLogs` | The redaction check itself. Skips values shorter than 8 chars to avoid false positives on short attributes. |
