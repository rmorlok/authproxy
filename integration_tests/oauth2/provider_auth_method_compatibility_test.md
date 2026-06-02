# Provider Auth Method Compatibility

Companion notes for `provider_auth_method_compatibility_test.go`.
Covers OAuth provider token-endpoint client authentication compatibility.

## Scope

The tests prove the proxy can exchange authorization codes with OAuth2
providers that require different token-endpoint client authentication methods:

- `client_secret_basic`
- `client_secret_post`
- `none` for public clients with PKCE
- a mismatched proxy/provider method that fails during token exchange

## Approach

These are non-interactive token-endpoint scenarios, so the tests use the
go-oauth2-server `/test/*` control plane rather than Chrome:

1. Register a provider client with the desired `token_endpoint_auth_method`.
2. Initiate a real AuthProxy OAuth2 connection.
3. Mint a real authorization code through `/test/authorize`.
4. Deliver the callback through the production public handler.
5. Inspect the provider's recorded `/token` request.

The public-client case follows `/oauth2/redirect` once before calling
`/test/authorize` so the provider binds the authorization code to the same PKCE
challenge the proxy generated.

## Assertions

Successful cases must persist an OAuth2 token, leave the connection configured,
and send the expected wire shape:

- Basic auth uses `Authorization: Basic ...` and does not duplicate credentials
  in the form body.
- Post-body auth sends `client_id` and `client_secret` in the form body and no
  Authorization header.
- Public-client auth sends `client_id`, omits `client_secret`, and includes a
  PKCE `code_verifier`.

The mismatched-method case registers the provider for `client_secret_basic` but
configures the proxy for `client_secret_post`. It must fail clearly with no
token row, an `auth_failed` setup step, a token-exchange failure log event, and
no plaintext client secret in logs or setup error.
