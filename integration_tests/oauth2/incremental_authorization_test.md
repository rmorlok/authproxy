# OAuth incremental authorization

This test verifies incremental authorization during OAuth re-authentication. It drives re-authentication through the marketplace UI and the provider's login/consent pages with chromedp, rather than short-circuiting through the provider test API.

The provider fixture currently accepts single scope identifiers in the browser flow, so this scenario uses `read_write` as the optional upgraded scope. The first token response is scripted to grant only `read`, which leaves the connection configured with a missing optional scope. Re-authentication then either returns `read_write` or fails at token exchange.

The assertions cover:

- the configured connection can still proxy requests while the user is paused at the upgrade consent page;
- pending re-authentication does not replace the stored token or granted scope set;
- successful re-authentication replaces the token row and updates granted scopes from `read` to `read_write`;
- failed re-authentication leaves the original token, granted scopes, and configured connection state intact while surfacing `auth_failed` as a retryable setup step.
