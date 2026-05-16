# OAuth2 Provider Timeouts (scenario 18)

Companion specification for `provider_timeouts_test.go`. Covers issue
#177 scenario 18 — when a provider endpoint hangs (or, more broadly,
when the connection terminates mid-flight), the proxy must classify the
failure as a transient transport error, retry on the legs where retry
is policy, and never silently overwrite credentials.

## Scope

Three tests, one per leg the proxy itself drives:

1. **`TestTokenExchangeTimeout_RetriesAndExhausts`** — token endpoint
   during the authorization-code → access-token exchange leg
   (`callback.go` → `postTokenExchangeWithRetry`).
2. **`TestTokenRefreshTimeout_RetriesAndExhausts`** — refresh leg
   (`proxy.go` → `postRefreshWithRetry`).
3. **`TestUpstreamApiTimeout_SurfacedToCaller`** — upstream API leg
   (`proxy.go` → `sendProxyRequest`).

Each test asserts (a) the proxy's response, (b) the structured log
event(s), (c) the connection's state and health, (d) the persisted
token row's integrity, and (e) the exact number of HTTP attempts the
proxy made.

## Why DropConnection (not a literal timeout)

The proxy has **no HTTP-level timeout configured**:

- `internal/httpf/factory.go` wraps `http.DefaultTransport` and never
  sets `Timeout` on the underlying client.
- `internal/schema/connectors/auth_oauth2_token.go`'s `RefreshTimeout`
  (default 30s) is used only by the Redis mutex (`lock.go` →
  `apredis.MutexOption*`), not by any HTTP client.

A scripted server-side `time.Sleep` therefore has no observable effect
on the proxy — the proxy would happily wait until the test framework
killed it. The closest faithful reproduction of a "provider hung and
the request died" failure is to terminate the connection mid-flight.
`helpers.ScriptAction{DropConnection: true}` does this: the test
provider's script middleware hijacks the `http.ResponseWriter` and
calls `Close()` on the underlying conn, producing an `EOF` /
`connection reset` on the proxy's pending POST or GET.

From the proxy's perspective, a server-side timeout, a TCP reset, and
a process crash are all the same failure: a transport error on the
pending request, with no HTTP status to classify. That is the
observable surface scenario 18 is about.

If and when the proxy adds an HTTP client timeout, the same tests
should still cover the property — the failure surfaces the same way
(`network_error` category, no `provider_status_code`).

## What each test pins

### `TestTokenExchangeTimeout_RetriesAndExhausts`

- 3 POSTs to `/token` (the full `tokenExchangeMaxAttempts` budget).
  `FailCount=10` outlasts the budget; the unused drops sit in the
  script queue.
- Exactly one `oauth token exchange failed` event with:
  - `category = network_error`
  - `attempts = 3`
  - `provider_status_code` field **absent** — there is no status code
    to record when the connection died before a response.
- Connection lands in `state=created`, `setup_step=auth_failed`,
  `setup_error` populated.
- No token row persisted.
- 302 redirect to `return_to_url?setup=pending&connection_id=<id>` so
  the marketplace UI re-renders the connection in its failed state.

### `TestTokenRefreshTimeout_RetriesAndExhausts`

- 3 POSTs to `/token` with `grant_type=refresh_token` (the full
  `tokenRefreshMaxAttempts` budget).
- Exactly one `oauth token refresh failed` event with
  `category=network_error`, `attempts=3`, no `provider_status_code`.
- 2 `oauth token refresh transient failure; retrying` warn lines (one
  before each retried attempt — the terminal attempt does not schedule
  a further retry).
- **Connection health stays `healthy`** and no
  `connection health state changed` event is emitted. This is the
  load-bearing distinction from the permanent refresh categories
  (`invalid_grant`, `invalid_client`, `provider_4xx_other`,
  `malformed_response`): network errors are transient, so the next
  proxy call gets another chance and dashboards shouldn't paint the
  connection as broken on a blip.
- Token row preserved byte-for-byte: same id, same
  `EncryptedRefreshToken` as snapshotted right after
  `forceTokenExpired`. A transport failure cannot leave partial bytes
  in the DB because the body is never parsed — this assertion is the
  proof.
- Proxy API returns non-200 to the caller (500 from `apgin.WriteErr`
  wrapping the transport error).
- No `oauth token refresh succeeded` event.

### `TestUpstreamApiTimeout_SurfacedToCaller`

- Exactly **one** upstream GET to `/test/resource/echo` since a
  snapshot timestamp captured before the proxy call. **No retry**: the
  proxy's resource leg only retries on a 401 response (the
  refresh-and-replay path), and a transport error has no status to
  trigger that. `sendProxyRequest` returns the error directly and
  `ProxyRequest` propagates it.
- **No refresh attempted** (0 refresh POSTs). The transport error must
  not be misclassified as a 401 — a non-zero refresh count here would
  mean the proxy mistook a connectivity failure for an auth failure.
- No `oauth token refresh failed` and no `oauth token refresh
  succeeded` event.
- Connection state stays `ready`, health stays `healthy`, no
  `connection health state changed` event. An upstream connectivity
  blip is not a credential problem.
- Proxy API returns non-200 to the caller.

## Cross-references

| Property | Where else covered |
| --- | --- |
| Token-exchange retry budget on 5xx | `callback_token_exchange_retry_test.go` (PR for #168). Same retry helper as the timeout case; 5xx exhaustion and timeout exhaustion both produce `attempts=tokenExchangeMaxAttempts`. |
| Refresh retry budget on 5xx | `proxy_refresh_retry_test.go` (PR for scenario 8). Asserts the parallel observable shape against 5xx; the 5xx test is the canonical case and this scenario is the transport-layer companion. |
| Transport-error retry at the helper level | `internal/auth_methods/oauth2/proxy_test.go::TestPostRefreshWithRetry_TransportErrorRetried` and `internal/auth_methods/oauth2/callback_test.go::TestPostTokenExchangeWithRetry_TransportErrorRetried`. Unit-level coverage of the same retry branch this scenario drives end-to-end. |
| Permanent vs transient health flip | `proxy_refresh_failure_test.go` (scenario 7) for the permanent categories; this file for the transient `network_error` category. |
| Wire-format response parsing on the refresh leg | `proxy_refresh_failure_test.go::MalformedResponse` row and `callback_token_exchange_malformed_test.go` (scenario 17). |

## What is *not* covered here

- **Authorize endpoint timeouts.** The proxy never POSTs to the
  authorize endpoint — the user's browser hits it directly. A hung
  authorize page manifests as a UI failure (the user sees a spinner
  forever, then closes the tab) and never reaches any proxy code path
  that scenario 18 could cover. The state row would expire on TTL.
- **Revocation endpoint timeouts.** Revocation is fired from the
  background `disconnect_connection` Asynq task in
  `internal/auth_methods/oauth2/revocation.go`; the integration-test
  harness doesn't run the worker by default, so reproducing a
  revoke-timeout end-to-end requires standing one up. The P2
  disconnect tests (issue #181) cover the disconnect path including
  revocation failure modes. The unit test
  `revocation_test.go::TestRevokeRefreshToken_*` pins the per-call
  classifier behavior.
- **Time-budgeted HTTP-client timeouts.** The proxy doesn't have one.
  When/if it does, the same tests cover the property — a client-side
  timeout surfaces as the same transport error as the server dropping
  the connection.
- **Partial-body / mid-response drops.** `DropConnection` runs before
  the response body is written, so the body-parser path is not
  exercised here. A drop mid-body would surface as an `io.ErrUnexpectedEOF`
  on the read; that is a different code path that the malformed-
  response scenario (#176) is the natural home for if needed.

## Components

| Lever | What it controls |
| --- | --- |
| `tokenExchangeRetryRig` + `initiateAndMintCode` | Per-test fixture for the exchange-leg test. Reused from `callback_token_exchange_retry_test.go`; provides `tokenCallCount()` for the attempt-count assertion. |
| `proxyRefreshRig` + `completeAuthFlow` + `forceTokenExpired` | Per-test fixture for the refresh and upstream tests. Reused from `proxy_refresh_test.go`; `refreshCallCount()` from `proxy_refresh_retry_test.go` counts `grant_type=refresh_token` POSTs. |
| `helpers.ScriptAction{DropConnection: true, FailCount: 10}` | The single primitive driving every failure in this file. `FailCount` is set well past the proxy's retry budget so the test exercises the exhaustion path on the retried legs and is robust to any future retry-budget bump on the upstream leg. |
| `helpers.RequestsFilter{Since: time.Now()}` | Snapshots a baseline timestamp before the upstream-test fires its proxy call. The test provider's recorder is process-global on the docker container — without a `since` cutoff, prior tests' `/test/resource/*` records would contaminate the "exactly one upstream call" count. The refresh and exchange tests use `ClientID` filtering for the same effect (each rig uses a unique per-test client key). |
| `provider.Script("", EndpointResource, …)` | Scripts on the wildcard (empty `clientID`) queue. Resource requests carry only a Bearer access token (no `client_id` form field, no Basic auth), so the test provider's `extractClientID` returns `""` for them — only the wildcard queue can match. |
| `tokenExchangeFailureMessage` / `tokenRefreshFailureMessage` / `tokenRefreshRetryMessage` / `connectionHealthStateChangedMessage` | Message strings pinned in the existing failure / retry test files. Used as `logCapture.RecordsWithMessage` haystacks. |
