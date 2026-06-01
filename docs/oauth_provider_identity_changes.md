# OAuth provider identity changes

Tracks the remaining work for issue #182 / OAuth test-suite scenario 27.

## Current status

AuthProxy does not currently consume provider account identity during OAuth setup or re-authentication. The OAuth callback exchanges the authorization code for tokens, validates/persists token material, and advances the connection, but it does not call a `userinfo` endpoint, parse an ID token, or store a provider subject on the connection/token record.

Because there is no stored provider identity and no connector-level stable-identity policy, an integration test cannot yet assert "reconnect as provider user B is rejected for a connection originally authorized as provider user A". The product has no behavior to exercise.

The current data model confirms this gap: `oauth2_tokens` stores encrypted access/refresh tokens, expiry, requested/granted scopes, and the AuthProxy actor that initiated the token, but not provider `sub`, email, account id, tenant id, or an identity verification timestamp.

## Required product behavior

To support the scenario, AuthProxy needs an explicit provider-identity contract:

- Connector schema for identity source:
  - `userinfo.endpoint`, or an ID-token claim mapping if OIDC-style responses are supported.
  - Stable identity key, usually `sub`, with optional display fields such as `email`.
  - Policy knob for re-authentication mismatch, for example `stable_identity: required`.
- OAuth callback behavior:
  - Fetch or parse provider identity after successful token exchange when configured.
  - Store the provider identity with the connection or active token.
  - On re-auth for an already configured connection, compare the new identity with the previously stored identity before replacing credentials.
- Failure policy:
  - If stable identity is required and the provider identity changes, reject the re-auth attempt.
  - Preserve the existing configured connection and working credentials.
  - Surface a retryable `auth_failed` setup step with an error that names the identity mismatch category without leaking tokens.

## Required provider behavior

The test provider also needs enough surface area to drive the user-facing scenario through chromedp:

- A userinfo/profile endpoint returning at least `sub` and `email` for the currently authorized token.
- Request inspection for userinfo calls.
- Ability to create two provider users and log in as user A, then log out or switch session and log in as user B during the re-auth browser flow.
- Test-mode controls to mutate identity fields remain useful for lower-level tests, but the issue's preferred approach is browser-driven because the login/consent leg is part of the scenario.

## Target integration test

Once the product and provider capabilities exist, add a chromedp test under `integration_tests/oauth2`:

1. Register an OAuth connector with stable provider identity required.
2. Connect through the marketplace as provider user A.
3. Assert the connection is configured, token is stored, and provider identity is recorded as A.
4. Start re-authentication from the marketplace.
5. Log in and consent as provider user B.
6. Assert the re-auth is rejected as an identity mismatch.
7. Assert the original token row and configured connection remain active.
8. Assert a subsequent proxy call still uses the original credentials.

This should close #182 once the prerequisite behavior exists.
