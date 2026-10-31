# Auth Methods

This package contains AuthProxy's authentication implementations. Each method
resolves credentials for one connection and describes how to apply them to a
request. The proxy handles sending and its existing single recovery attempt
after an upstream 401. Methods may also persist credentials and refresh tokens.

## Credential failures

Authentication implementations use `auth_methods.MarkCredentialsUnavailable(err)`
when they know a connection's credentials are absent or unusable. Internal
callers can check `errors.Is(err, auth_methods.ErrCredentialsUnavailable)` without
parsing diagnostic text or guessing from an HTTP status.

For example, a missing row from `GetActiveApiKeyCredential` is marked. A
decryption error while reading an existing row is not: the encryption key or
service may be unavailable even though the connection's credential still works.

| Failure at its source | Marked unavailable? |
| --- | --- |
| Missing active API key, OAuth token, or stored OAuth client credentials | Yes |
| Expired OAuth access token with no required refresh token | Yes |
| Explicit OAuth refresh `invalid_grant`, including after an ambiguous attempt | Yes |
| Database, decryption, malformed stored data, or connector configuration failure | No |
| OAuth `invalid_client`, unrecognized 4xx, or malformed token response | No |
| Refresh network failure, 5xx, or cancellation | No |

The marker preserves the original error text, cause chain, and HTTP-aware
status/message for existing proxy callers. Its underlying diagnostics remain
private. The Tool HTTP host converts a returned marked error to its own fixed
`ErrCredentialsUnavailable`, discarding the cause.

This category is independent of connection health and target-send evidence.
Some configuration/provider failures mark a connection unhealthy but do not
prove that reconnecting would fix it. An unavailable-credential error can also
follow an earlier target request that may already have caused effects.

Failed recovery still returns the original upstream 401 response through the
proxy, regardless of the refresh failure's category. That response remains
available to Tool status rules; the host does not infer this marker from a 401.
