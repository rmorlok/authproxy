# Auth Methods

This package supplies authentication for outbound proxy requests. The root
package defines shared interfaces and errors; subpackages implement each
method. The [proxy](../proxy) builds and sends requests using their credentials.

| Implementation | What it provides |
| --- | --- |
| [`oauth2`](oauth2) | Access tokens, token refresh, authorization/setup flows, and optional revocation. |
| [`api_key`](api_key) | Stored API keys applied as bearer tokens, headers, query parameters, or HTTP Basic credentials. |
| [`no_auth`](no_auth) | An empty credential application; caller-supplied headers and query parameters stay intact. |

## Shared interfaces

Core selects a [`Factory`](factory.go) for the connector's authentication type.
`NewAuthenticator(connection)` creates its credential handler.
`ManifestSetupSteps` supplies authentication setup steps, such as an API-key
form or an OAuth authorization redirect.

The resulting [`Authenticator`](authenticator.go) has five operations:

| Method | Purpose |
| --- | --- |
| `Resolve(ctx)` | Load usable credentials for one request attempt, refreshing expired OAuth tokens when needed. |
| `RecoverFrom401(ctx, rejected)` | Try to replace the credentials rejected by the upstream. |
| `Refresh(ctx)` | Renew credentials during maintenance, without an upstream 401. |
| `SupportsRevoke()` | Report whether remote credential revocation is supported. |
| `Revoke(ctx)` | Revoke credentials and clear their stored token when supported. |

API-key and no-auth implementations have nothing to refresh or revoke, so those
operations return nil. OAuth revocation requires a configured endpoint; callers
check `SupportsRevoke()` before invoking it.

## Credential data

`Resolve` returns an `AuthApplication` for one request attempt. For example,
OAuth can return:

```go
auth_methods.AuthApplication{
    Headers: map[string]string{"Authorization": "Bearer example-token"},
    CredentialVersion: "opaque-token-row-id",
}
```

An API key placed in the URL instead uses `QueryParams`:

```go
auth_methods.AuthApplication{
    QueryParams: map[string]string{"api_key": "example-key"},
}
```

The proxy sets these headers and query parameters after caller input, replacing
matching values and preserving other fields. Query values are raw strings;
the proxy performs URL encoding. The maps contain secrets and must stay out of
Tool results and ordinary logs.

`CredentialVersion` is an opaque identifier, not sent upstream. OAuth uses it to
detect whether another request already refreshed a rejected token. Methods
without versioned credentials leave it empty.

## Request lifecycle

1. The proxy calls `Resolve`, applies the returned headers/query values, and sends.
2. If the response is 401 and the request can be replayed, it calls
   `RecoverFrom401` with the rejected `AuthApplication`.
3. If recovery succeeds, it calls `Resolve` again and retries once. Failed
   recovery leaves the original 401 available to the caller, unless canceled.

Streaming requests need either no body or a `GetBody` function that creates a
fresh body for replay. API keys are loaded on every `Resolve`, so a replacement
key takes effect on the next attempt.

## Error contract

`ErrCannotRecover` means the method cannot automatically recover from a 401.
API-key and no-auth handlers return it.

`ErrCredentialsUnavailable` means credentials are known to be missing or
unusable. Implementations add it with `MarkCredentialsUnavailable(err)`;
callers check it with `errors.Is`. Examples include a missing stored API key,
an expired OAuth token with no required refresh token, or an explicit
`invalid_grant` response during refresh.

Database, decryption, configuration, or transient refresh failures do not prove
credential loss. Neither does a generic 401 or unhealthy connection. The marker
preserves the original error and cause; the [Tool HTTP host](../tools/httphost)
exposes only a fixed category to keep diagnostics private.

Credential failure does not prove that earlier requests were unsent. Callers
track possible upstream effects separately when deciding whether to retry.
