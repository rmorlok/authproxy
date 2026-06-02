# Proxy API Response Handling

Companion notes for `proxy_response_handling_test.go`.
Covers proxied upstream response handling for successful responses, auth failures, permission failures, rate limits, and server errors.

## Scope

The suite verifies OAuth-backed proxy behavior once a connection already has
valid provider credentials:

- 200 responses are returned without mutation and include the expected Bearer
  token upstream.
- 401 responses trigger one forced refresh and one replay.
- Persistent 401 responses surface after that single retry, avoiding loops.
- 403 responses are treated as authorization/permission failures, not refresh
  prompts.
- 429 responses are treated as upstream rate limits, preserving `Retry-After`
  and avoiding OAuth reconnect/refresh behavior.
- 5xx responses are surfaced as upstream failures without OAuth refresh.

## Approach

The tests reuse `proxyRefreshRig`:

1. Complete a real authorization-code flow through `/test/authorize`.
2. Script the go-oauth2-server resource endpoint with the response under test.
3. Call AuthProxy's wrapped `/api/v1/connections/{id}/_proxy` path.
4. Inspect both the wrapped proxy response and the provider's request recorder.

The resource endpoint is scripted through the wildcard client key because
resource requests authenticate with Bearer tokens, not client credentials.

## Observability

The proxy orchestrator emits stable structured events for the upstream response
classifications:

- `proxy upstream retry attempted`
- `proxy upstream auth failure`
- `proxy upstream rate limited`

The tests assert these events fire only for the final response category that
reaches the caller. A 401 that self-heals therefore records a retry attempt and
refresh success, but no final auth-failure event.
