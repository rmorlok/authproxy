# OAuth2 One-Time-Use Refresh Tokens

Companion specification for `proxy_refresh_one_time_use_test.go`. Covers
issue #1002: providers (Atlassian, GitHub Apps, Box, Intuit, …) that rotate
the refresh token on every refresh and invalidate the one that was redeemed.

The docker test provider behaves the same way with rotation on (its test-mode
default): replaying a rotated-away refresh token returns
`400 "Refresh token revoked"`. Any code path that redeems the same refresh
token twice therefore shows up as a refresh-failed event and an unhealthy
connection, which is exactly what these tests assert never happens.

## Cases covered

| Test | Setup | Asserted shape |
| --- | --- | --- |
| `TestOneTimeUseRefresh_ConcurrentUpstream401sRotateOnce` | Access token revoked at the provider while it still looks valid locally; 8 concurrent proxy calls all get 401 from `/echo`. Refresh slowed by 300ms so the 401s pile up. | Every call ends 200 upstream. Exactly **one** refresh grant, one success event, no failure events, connection healthy. |
| `TestOneTimeUseRefresh_RefreshSlowerThanLockLease` | `refreshTimeout: 1s` (lock lease). Refresh scripted as 600ms → 503, then 600ms → real rotation (~1.4s total). 8 concurrent proxy calls with an expired access token. | Every call ends 200 upstream. Exactly two refresh POSTs (the 503 and its retry), one success event, no failure events, connection healthy. |

## What each case guards

- **Concurrent 401s.** Each request hands the token it used back to
  `RecoverFrom401`. Only the first recovery refreshes; the rest see the
  rejected token was already replaced and retry with the new one. Without
  this, every 401 forced its own refresh (8 grants for one revocation). This
  test was verified to fail with that check disabled.
- **Refresh slower than the lease.** The refresh lock's lease is renewed for
  the whole refresh. If it lapsed at 1s, a waiting request would take the lock,
  re-read the not-yet-replaced token and replay its refresh token. Waiters also
  poll the token row rather than queueing for the lock, so all 8 calls finish
  shortly after the refresh instead of draining through the lock about once
  per second.

## Components

| Lever | What it controls |
| --- | --- |
| `newProxyRefreshRigWithOptions` | Same rig as `proxy_refresh_test.go`, with a hook to set `OAuth2ConnectorOptions.RefreshTimeout`. |
| `provider.RevokeToken(accessToken)` | Revokes only the access token, so `/echo` returns 401 while the refresh token stays valid. |
| `provider.Script(…, EndpointRefresh, …)` | Delay-only actions fall through to the real rotation handler; the 503 action does not consume the refresh token. |
| `refreshGrantRequests(rig)` | Counts `grant_type=refresh_token` POSTs observed by the provider. |
| `logCapture.RecordsWithMessage` | Success / failure / health-transition events. |

## Related

- `proxy_refresh_rotation_test.md`: rotation persistence and forward chain.
- `proxy_refresh_concurrent_test.go`: lazy-refresh concurrency at default timeouts.
- `refresh_token_expiry_test.go`: refresh-token lifetime fields (`refresh_token_expires_in` and variants).
- Unit coverage with an in-process one-time-use provider:
  `internal/auth_methods/oauth2/refresh_rotation_test.go`.
