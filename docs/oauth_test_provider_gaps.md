# OAuth Test Provider — Gap Analysis

Tracks the gap between [`rmorlok/go-oauth2-server`](https://github.com/rmorlok/go-oauth2-server) (the controllable fake third-party OAuth provider used for AuthProxy integration tests) and the OAuth integration test scenarios specified in issue [#159](https://github.com/rmorlok/authproxy/issues/159).

This document is the deliverable for issue [#163](https://github.com/rmorlok/authproxy/issues/163).

## Executive summary

The upstream `go-oauth2-server` (and its `rmorlok` fork) is a real, spec-compliant OAuth2 server. It implements the canonical happy-path flows but was not built as a test harness — it lacks the scripting hooks, error injection, PKCE, revocation, refresh-rotation, and resource-server simulation we need.

The full P0 / P1 / P2 test matrix in #159 cannot be implemented against the current upstream. We have three options for closing the gap; the recommended path is **Option B — extend the `rmorlok` fork with a test-mode control plane**. The remainder of this document enumerates every gap and feeds the fork-extension plan.

### Recommendation

**Adopt Option B**: extend the `rmorlok` fork with a `--test-mode` flag that exposes a control-plane HTTP API (`/test/...`) and adds the missing OAuth surface area (PKCE, revocation, refresh rotation, scriptable behavior, a stub resource server). Land the changes upstream-of-AuthProxy as one or more PRs to `rmorlok/go-oauth2-server`; consume them from AuthProxy via a tagged release.

Rationale: Option A blocks 17 of 30 spec scenarios; Option C duplicates a real OAuth server we already control.

### Options considered

| Option | Description | Pros | Cons |
|---|---|---|---|
| A — Use as-is | Limit the test suite to what the current server supports. | No upstream work. | Blocks ~17/30 scenarios including most P0 robustness/refresh tests. |
| **B — Extend the fork** *(recommended)* | Add a test-mode control API and missing OAuth features to `rmorlok/go-oauth2-server`. | Real OAuth code paths; reusable for other projects; minimal duplication. | Upstream PRs needed before tests can begin. |
| C — Purpose-built fake | Build a new in-process fake tailored to our matrix. | Complete control; no external dependency. | Re-implements OAuth correctness; fake-vs-real divergence risk. |

## Required server capabilities (from #159)

| Capability | Current support | Action |
|---|---|---|
| Successful authorization code flow | ✅ | — |
| User approval and rejection | ✅ (`allow` form param) | Add programmatic control instead of HTML form (see G2). |
| Configurable granted scopes | ⚠️ Partial — server returns the requested scope only | G3 |
| Authorization code expiration | ✅ (configurable lifetime) | — |
| Authorization code replay failure | ✅ (single-use, deleted on exchange) | — |
| Token exchange success and failure | ✅ for happy path; ❌ for scripted failure modes | G4 |
| Refresh token success and failure | ✅ happy; ❌ scripted | G4 |
| Refresh token rotation | ❌ — `GetOrCreateRefreshToken` reuses existing tokens | G5 |
| Token revocation | ❌ — only introspection (`/v1/oauth/introspect`), no `/revoke` | G6 |
| Configurable transient failures | ❌ | G4 |
| Configurable malformed responses | ❌ | G4 |
| Configurable upstream API responses | ❌ — no resource server | G7 |
| Request inspection (auth headers, scopes, tokens) | ❌ | G8 |

## Required-changes table

Per #159, each blocked capability is recorded with priority, required behavior, blocked tests, and a proposed change.

| Priority | ID | Missing Capability | Required Behavior | Blocked Tests | Proposed Change |
|---|---|---|---|---|---|
| P0 | G1 | Headless test bootstrap | Provider must start without etcd/consul and accept dynamic client/user registration over an admin/control API. | All — server currently requires etcd or consul plus Postgres just to start. | Add `--test-mode` that uses an embedded SQLite store, skips remote config, and exposes `POST /test/clients`, `POST /test/users`. |
| P0 | G2 | Programmatic authorization decision | Drive approve/deny without rendering HTML / managing a session cookie. | #164, #165, #166, #167 (and any test driving the auth flow). | Add `POST /test/authorize` that takes `{ client_id, user_id, redirect_uri, scope, state, decision: "approve"\|"deny", granted_scope?, code_challenge?, code_challenge_method? }` and returns the redirect URL the test should hand the proxy. |
| P0 | G3 | Configurable granted scopes per authorization | Provider can grant scopes that differ from requested scopes (subset, additional, or scope omitted in token response). | #166 — scope mismatch | Honour `granted_scope` in `POST /test/authorize` (G2). Add `omit_scope_in_token_response` toggle on the scripted token response (G4). |
| P0 | G4 | Scriptable token / refresh / revocation responses | Per-client (or per-flow) script: success, transient 5xx with attempt count, permanent error code (`invalid_grant`, `invalid_client`), malformed body (invalid JSON, wrong content type, missing fields, bad expiry, etc.), timeout. | #168 (code exchange failures), #170 (refresh failures + transient retry), #175 (malformed callbacks), #176 (malformed token responses), #177 (timeouts), #173 (log redaction — needs failure paths). | Add `POST /test/scripts` with a queue of responses keyed by client and endpoint (`token`, `refresh`, `revoke`). Each entry: `{ status, headers, body_raw \| body_template, delay_ms, fail_count_before_success }`. |
| P0 | G5 | Refresh token rotation | Refresh response may include a new refresh token; old one is invalidated. | #171 — rotation + concurrent refresh | Add `oauth.refresh_token_rotation: true` config or per-client flag. On rotation, return a new `refresh_token` and mark the old one revoked at the data layer so #171 concurrency tests can observe staleness. |
| P0 | G6 | Token revocation endpoint | `POST /v1/oauth/revoke` per RFC 7009; revoked access/refresh tokens are immediately invalid for token, refresh, and resource calls. | #172 (third-party revocation), #181 (proxy-initiated disconnect), parts of #170 (refresh on revoked token). | Implement `RFC 7009` revocation handler. Plus `POST /test/revoke` for tests that need to simulate provider-side revocation without the client calling the endpoint. |
| P0 | G7 | Resource server simulation | Provider exposes protected test endpoints that validate the bearer token (200 / 401 / 403 / 429 / 5xx) and can be scripted per request. | #179 (proxy API response handling), and any test that drives a request through the proxy. | Add `/test/resource/...` endpoints that validate the bearer token and return responses driven by the same script queue from G4. |
| P0 | G8 | Request inspection | Tests can read what the proxy actually sent (headers, query, form body) to authorize, token, refresh, revoke, and resource endpoints. | #167 (state security — needs to assert no token call), #173 (log redaction cross-checks), #178 (auth method compatibility), #179 (proxy injection). | Record requests in memory; expose `GET /test/requests?endpoint=token&since=...` that returns redacted-but-asserted-on metadata: timestamp, method, path, headers (with `Authorization` shape), query params, form fields. |
| P1 | G9 | PKCE validation | Server validates `code_challenge` / `code_challenge_method` at authorize time and `code_verifier` at token exchange. | #174 — PKCE validation | Add PKCE per RFC 7636 (`S256` and `plain`). Tests that need a missing-verifier path should be able to skip the PKCE binding via a script flag. |
| P1 | G10 | Auth method compatibility | Token endpoint accepts client credentials via Basic auth, body params, or rejects depending on configured method; supports public clients (no secret) with PKCE. | #178 — auth method compatibility | Add `client.token_endpoint_auth_method: client_secret_basic \| client_secret_post \| none`. Reject mismatches with `invalid_client`. |
| P2 | G11 | Account identity simulation | Provider can return stable or changed userinfo/profile responses across reconnects. | #182 — identity changes | Add `/v1/oauth/userinfo` endpoint or extend introspection with `sub` and `email`. Allow the test mode to swap the user bound to a refresh token between authorizations. |
| P2 | G12 | Multi-tenant client isolation | Multiple clients with disjoint redirect URIs and refresh-token namespaces. | #183 — multiple connections | Already largely supported via per-client config; verify isolation in tests. May need test-mode helpers to register multiple clients quickly (covered by G1). |

## Per-scenario blocking matrix

| Spec | Sub-issue | Status against current upstream | Blocking gaps |
|---|---|---|---|
| #1 Successful flow | #164 | Blocked on harness | G1, G2 |
| #2 User denial | #165 | Blocked | G1, G2 |
| #3 Scope mismatch | #166 | Blocked | G1, G2, G3 |
| #4 Callback state security | #167 | Blocked | G1, G2, G8 |
| #5 Code-exchange failures | #168 | Blocked | G1, G4 |
| #6 Expiry + refresh / #13 no-refresh | #169 | Blocked | G1, G2 |
| #7 Refresh failures / #8 transient retry | #170 | Blocked | G1, G4 |
| #9 Rotation / #10 concurrent refresh | #171 | Blocked | G1, G5 |
| #11 Third-party revocation | #172 | Blocked | G1, G6 |
| #12 Sensitive values not logged | #173 | Blocked transitively (needs working flows) | G1, G2, G4 |
| #14 PKCE | #174 | Blocked | G1, G2, G9 |
| #15+#16 Malformed callbacks | #175 | Blocked | G1, G2 |
| #17 Malformed token responses | #176 | Blocked | G1, G4 |
| #18 Timeouts | #177 | Blocked | G1, G4 |
| #19 Auth method compat | #178 | Blocked | G1, G10 |
| #20–#24 Proxy API responses | #179 | Blocked | G1, G4, G7 |
| #25 Incremental auth | #180 | Blocked | G1, G2 |
| #26 Proxy-initiated disconnect | #181 | Blocked | G1, G6 |
| #27 Identity changes | #182 | Blocked | G1, G11 |
| #28 Multiple connections | #183 | Blocked | G1 (and G12 verification) |
| #29 Open redirect | #184 | Blocked on harness | G1 |
| #30 Clock skew | #185 | Blocked on harness | G1 |

Net: every test sub-issue depends on G1 (headless test bootstrap). G2 and G4 unblock the bulk of the matrix.

## Suggested upstream PR sequencing

Pull requests against `rmorlok/go-oauth2-server`, in dependency order:

1. **PR-1: test-mode bootstrap** — `--test-mode`, embedded SQLite store, no etcd/consul dependency, `POST /test/clients`, `POST /test/users`, `GET /test/health`. Closes G1.
2. **PR-2: programmatic authorize + request log** — `POST /test/authorize`, `GET /test/requests`. Closes G2 and G8. Lights up the happy-path flow + denial.
3. **PR-3: scriptable responses** — `POST /test/scripts` for `token`, `refresh`, `revoke`. Closes G4. Unblocks the failure-mode P0/P1 tests.
4. **PR-4: refresh rotation** — config flag; rotate-on-refresh and revoke-old behavior. Closes G5.
5. **PR-5: revocation endpoint** — `POST /v1/oauth/revoke` (RFC 7009) + `POST /test/revoke`. Closes G6.
6. **PR-6: resource server stub** — `/test/resource/...`. Closes G7.
7. **PR-7: PKCE** — `code_challenge` / `code_verifier` (S256, plain). Closes G9.
8. **PR-8: auth method compatibility** — per-client `token_endpoint_auth_method`. Closes G10.
9. **PR-9: identity / userinfo** — closes G11. (P2; can ship later.)

After PR-1 through PR-3 land, the harness in [#162](https://github.com/rmorlok/authproxy/issues/162) can begin and a useful subset of P0 tests can start.

## Tracking

Upstream issues will be filed against `rmorlok/go-oauth2-server` once this analysis is approved. Each upstream issue links back here and to the AuthProxy sub-issue(s) it unblocks.
