# OAuth open redirect protection

Companion specification for `open_redirect_test.go`. Covers issue #184 /
#159 scenario 29: OAuth callback and post-auth redirect behavior must not send
users to arbitrary external URLs.

## Coverage

`TestOAuth2OpenRedirectProtection_InvalidReturnURLFallsBackToMarketplace`
boots a real marketplace session in Chrome, initiates an OAuth connection from
that browser session with a malicious external `return_to_url`, and then drives
the provider login and consent forms through chromedp. After the provider
redirects back to AuthProxy's OAuth callback, the browser must land on the
marketplace `/connections` page, not the hostile origin.

The test also asserts the connection completed successfully and a token row was
persisted. That proves the safe fallback affects only the browser destination;
it does not abort an otherwise valid OAuth authorization.

## Product rule

OAuth return URLs are accepted only when they are absolute `http` or `https`
URLs on the configured public service origin. Malformed URLs, relative URLs,
URLs with embedded userinfo, unsafe schemes, and different origins are replaced
with the safe default: the public service's `/connections` page.
