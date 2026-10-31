# Wrapped proxy send evidence

Companion notes for `wrapped_send_evidence_test.go`. The public contract is
documented in [Proxying Requests](../../docs/src/content/docs/sdks/proxying.md).

## Approach

The suite starts AuthProxy's production API routes with
`helpers.Setup(StartHTTPServer: true)`, real middleware, and the selected SQLite
or PostgreSQL database. Requests cross real TCP sockets on both sides of the
proxy. AuthProxy runs in the test process; this is not a packaged-server or
deployment test.

`helpers.ControlledUpstream` records complete incoming request bodies, methods,
paths, and headers independently of AuthProxy. Each path can return a chosen
status/body, close the socket before headers, advertise an incomplete body, or
wait on channels. Fixture state is isolated per test and synchronized for
concurrent use.

## Coverage

| Scenario | Expected evidence | Independent check |
|---|---|---|
| Target 200, 401, 403, 429, or 500 | `sent` | Exactly one recorded POST with its original body; outer API status remains 200. |
| Provider body/header claims `notSent` | `sent` at the top level | Provider content remains nested and cannot override trusted evidence. |
| Missing/invalid authentication, missing permission, malformed payload, invalid method/ID, or missing connection | `notSent` | Error envelope and previous outer status remain intact; upstream receives nothing. |
| Provider 429 followed by local backoff | First `sent`, then `notSent` | Reconfigure the provider to 200; the second 429 still has no new upstream request. |
| POST recorded, then socket closed before response headers | `mayHaveSent` | The upstream received the complete operation body despite uncertainty at the caller. |
| Invalid upstream JSON after response headers | `sent` | The API returns its existing decoding error. |
| Truncated non-JSON response | `sent` | The API retains the wrapped body conversion's existing behavior. |
| Redirect followed by a dropped response | `sent` | The initial redirect and subsequent failed hop are separately recorded. |
| Redirect followed by local backoff | `sent` | A coordinated second request primes real rate limiting; the redirected hop never reaches upstream. |
| Sequential and overlapping requests on one connection | Independent states | Success, uncertain failure, and local rejection retain their own evidence. |

The redirect/drop case closes the redirect's connection before following the
next hop. A 302 changes the POST into a GET, and a reused connection could
otherwise trigger Go's transparent retry after a dropped response. Direct drop
cases use POSTs with bodies. Neither scenario depends on arbitrary sleeps or
external DNS/TLS behavior.

## Limits

These assertions concern evidence visible to AuthProxy, not proof of provider
side effects or safe retryability. The dropped-response case intentionally
demonstrates that `mayHaveSent` includes operations received by the provider.
No response can be inspected after the calling client disconnects; cancellation
and panic behavior are covered by lower-level tracking tests.

The current wrapped non-JSON body conversion discards read errors from
Gentleman's `Bytes()` helper. This suite preserves that behavior rather than
changing error semantics as part of evidence exposure. Raw streaming failures
and header timing belong to the raw proxy suite.

OAuth refresh/recovery cases live in
[`../oauth2/proxy_send_evidence_test.go`](../oauth2/proxy_send_evidence_test.go).
The integration README contains focused commands for both database providers.
