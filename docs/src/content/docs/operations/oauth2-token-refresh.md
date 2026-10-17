---
title: OAuth2 Token Refresh
description: How AuthProxy keeps OAuth2 connections authenticated, including providers that rotate one-time-use refresh tokens or expire refresh tokens.
---

AuthProxy refreshes OAuth2 access tokens for you, so applications proxying
through a connection never handle tokens. This page explains when refreshes
happen, how AuthProxy handles providers whose refresh tokens can be used only
once or expire, how to tune refresh, and how to read refresh failures.

## When AuthProxy refreshes

A connection's token is refreshed in three situations:

| Trigger | What happens |
|---|---|
| **Expired on use** | A proxied request finds the stored access token expired, refreshes it, then sends the request. |
| **Upstream 401** | The provider rejects a request with 401. AuthProxy refreshes once and replays the request. If the refresh fails, the original 401 is returned. |
| **Background task** | Every 10 minutes (`oauth.refreshTokensCronSchedule`), AuthProxy refreshes tokens whose access token expires within the connector's `refreshTimeBeforeExpiry` (falling back to `oauth.refreshTokensTimeBeforeExpiry`, default 10 minutes), and tokens whose refresh token is about to expire (see [Refresh-token expiry](#refresh-token-expiry)). Disable it globally with `oauth.refreshTokensInBackground: false`, or for one connector with `token.refreshInBackground: false`. |

The grant used is `refresh_token`, or `client_credentials` for connectors with
`grantType: client_credentials`. Client-credentials connectors simply request a
new token and ignore any refresh token in the response.

## One-time-use (rotating) refresh tokens

Many providers issue a new refresh token on every refresh and invalidate the
one that was just used. Examples include Atlassian, GitHub Apps, Box and
Intuit. Reusing an old refresh token fails with `invalid_grant`, and the user
must reconnect. AuthProxy supports these providers with no connector
configuration:

- **One refresher per connection.** Refreshes take a per-connection Redis lock,
  and the lock's lease is renewed for as long as the refresh runs. A slow
  provider or a retried request can't let the lock lapse and let a second
  process redeem the same refresh token.
- **Waiters reuse the result.** Requests that arrive during a refresh wait for
  it, then use the new token without calling the provider themselves.
- **One refresh per rejected token.** When many in-flight requests get 401 for
  the same access token, only the first triggers a refresh. The others retry
  with the replacement token.
- **The new token is always stored.** Once the provider returns a token,
  AuthProxy retries the database write if it fails, even if the request that
  triggered the refresh has been cancelled. Losing that token would otherwise
  strand the connection.
- **Rotation is followed.** Each refresh stores the refresh token the provider
  returned. If the response has no refresh token, the previous one is kept.

One case can't be prevented: AuthProxy retries token requests that fail with
a network error or a 5xx response. If the provider actually processed a
request whose response was lost, it has already rotated the refresh token, so
the retry fails with `invalid_grant`. AuthProxy reports this as
`invalid_grant_after_ambiguous_attempt` (see
[Failure categories](#failure-categories)), so you can tell it apart from a
user revoking access.

## Refresh-token expiry

OAuth 2.0 ([RFC 6749](https://www.rfc-editor.org/rfc/rfc6749#section-5.1))
only defines a lifetime for the access token. Some providers also report when
the refresh token expires. AuthProxy reads all of these fields:

| Token response field | Used by |
|---|---|
| `refresh_token_expires_in` | [GitHub Apps](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/refreshing-user-access-tokens), [LinkedIn](https://learn.microsoft.com/en-us/linkedin/shared/authentication/programmatic-refresh-tokens), [Google time-based access](https://developers.google.com/identity/protocols/oauth2/web-server) |
| `x_refresh_token_expires_in` | [Intuit QuickBooks Online](https://help.developer.intuit.com/s/article/Validity-of-Refresh-Token) |
| `refresh_token_timeout` | [draft-ietf-oauth-refresh-token-expiration](https://datatracker.ietf.org/doc/draft-ietf-oauth-refresh-token-expiration/) |

When a response includes one of these fields, AuthProxy records when the
refresh token expires and schedules a background refresh before then, so the
connection gets a new or renewed refresh token even if nobody uses it. If
several fields are present, the earliest deadline wins.

Some providers, such as LinkedIn, fix the refresh token's lifetime at the
original authorization, and refreshing doesn't extend it. When a refresh shows
the deadline didn't move, AuthProxy stops scheduling early refreshes for that
token. The user will need to reconnect when it expires.

## Configuration

Refresh settings live on the connector's `auth.token` block:

```yaml
spec:
  definition:
    auth:
      type: OAuth2
      token:
        endpoint: https://provider.example.com/oauth/token
        refreshTimeout: 30s
        refreshInBackground: true
        refreshTimeBeforeExpiry: 10m
        refreshTimeBeforeRefreshTokenExpiry: 24h
```

- `refreshTimeout` (default `30s`) is the deadline for each token-endpoint
  request, including reading the response. It also sets the lease on the
  per-connection refresh lock. The lease is renewed while a refresh runs, so
  it only controls how quickly a crashed process's lock is reclaimed.
- `refreshInBackground` (default `true`): when `false`, the background task
  doesn't refresh this connector's tokens ahead of time, neither before access
  tokens expire nor before refresh tokens expire. Tokens are still refreshed
  when they're found expired on use or after an upstream 401. The global
  `oauth.refreshTokensInBackground` must also be enabled for background
  refresh to run at all.
- `refreshTimeBeforeExpiry` sets how long before the access token expires the
  background task refreshes this connector's tokens. If unset, the global
  `oauth.refreshTokensTimeBeforeExpiry` (default `10m`) applies. Each
  connection uses the value from its own connector generation.
- `refreshTimeBeforeRefreshTokenExpiry` (default `24h`) sets how long before a
  reported refresh-token expiry to refresh. It is capped at half the refresh
  token's remaining lifetime.

## Failure categories

Every failed refresh logs an `oauth token refresh failed` event with a
`category`. The same value is the `reason` on the
`authproxy.oauth2.refresh.failures.total` metric (see
[Telemetry](/operations/telemetry/)). **Permanent** failures mark the
connection unhealthy so users are prompted to reconnect. **Transient** failures
leave health unchanged, and the next request or background run tries again.

| Category | Kind | Meaning |
|---|---|---|
| `invalid_grant` | Permanent | The provider rejected the refresh token as revoked, expired or already used. Usually the user revoked access or the token aged out. |
| `invalid_grant_after_ambiguous_attempt` | Permanent | `invalid_grant` after an earlier attempt for the same token may have reached the provider without AuthProxy seeing a response (dropped connection, timeout or 5xx). With rotating refresh tokens, that lost attempt most likely used up the token. Look into network or provider problems rather than user action. |
| `invalid_client` | Permanent | The client ID or secret is wrong. Fix the connector configuration. |
| `provider_4xx_other` | Permanent | Some other 4xx response, such as a WAF or rate-limiter page. |
| `malformed_response` | Permanent | The provider returned 200, but the body couldn't be parsed or had no `access_token`. |
| `no_refresh_token` | Permanent | The access token expired and no refresh token is stored. |
| `provider_5xx` | Transient | The provider returned 5xx after retries. |
| `network_error` | Transient | The token request failed in transit after retries. |
| `internal_error` | Transient | A problem on AuthProxy's side: decryption, timing out while waiting for a concurrent refresh, or failing to store a token the provider issued. Storage failures also log `failed to persist oauth token issued by provider` at error level. |
