# OAuth2 Callback State Security — Cross-Actor Case

Companion specification for `callback_actor_mismatch_test.go`. Covers
the cross-actor callback attack: an attacker initiates a connection, drives the
provider's authorize step to mint a code, and sends the resulting
`/oauth2/callback?state=…&code=…` URL to a different actor (the
victim). When the victim's browser follows the link, the public
service identifies the victim from their `SESSION-ID` cookie, but the
state record carries the attacker's actor id. State validation must
reject with `actor_mismatch` and redirect to the configured error
page; no token must be exchanged or persisted.

The direct-HTTP callback-shape and state-envelope rejection cases live in
`callback_state_security_test.go`. The cross-tenant and cross-connection
namespace defenses live in `callback_cross_namespace_test.go` and
`callback_namespace_mismatch_test.go`.

## Threat model

The bug-bounty shape: an attacker partially completes an OAuth flow
under their own session, then sends the *forged callback URL* to a
victim — a phishing link, a chat message, an embedded image, anything
that causes the victim's browser to issue a GET to the proxy's
callback endpoint. The state envelope was minted under the attacker's
actor id; the victim's browser carries the victim's session cookie.
If the proxy did not bind state-to-actor, the callback would attach
the attacker-controlled provider account to the victim's connection
record — a confused-deputy compromise.

## Why chromedp, not direct HTTP

Cases 1–4 hinge on programmatic tampering with the callback URL or
Redis envelope — values a real browser would never produce. Case 5
hinges on *who's calling*: the attacker minted the state under their
own actor id, but the request that delivers the code arrives carrying
the victim's session credentials. That delivery vector is exactly
what a phishing link looks like in the wild, and chromedp + the real
marketplace bootstrap is the closest possible reproduction:

- The victim's browser hits `/connectors?authToken=<victim JWT>`.
- The marketplace SPA calls `/api/v1/session/_initiate`, which mints
  a `SESSION-ID` cookie scoped to the victim.
- The browser then follows the forged callback URL. The cookie
  travels with the request; the public service identifies the
  caller as the victim.
- State validation reads the envelope, finds the attacker's actor id
  inside, compares against the calling actor (victim), and rejects.

A direct-HTTP path with a JWT signed as the victim would exercise the
same `state.ActorId` check — but it would not mirror the real-world
delivery vector. The chromedp path is what bug-bounty submissions
look like.

## What is asserted

- **Browser lands on the error page.** After the 302 from the proxy,
  the final URL exactly matches `ErrorPages.InternalError`, overridden
  through `SetupOptions.ConfigureRoot` to the local test server's
  `/500.html` URL. The fixture's unique `#oauth-callback-error` element
  is the load signal.
- **Exactly one `oauth callback rejected` log event** with
  `category=actor_mismatch` and `state_id` matching the minted state.
  The `actorId` field on the event reflects the *calling* actor
  (the victim) so a SOC analyst can see who clicked the link.
- **No `oauth2_tokens` row** exists for the connection.
- **Connection state is unchanged** — still `ConnectionStateSetup`, with
  `setup_step` at `OAuth2AuthorizeStepId` and `setup_error` nil.
- **Provider observed zero `/token` calls** for the test's client id.
  The token exchange was short-circuited by state validation.

## Components

| Lever                                                       | What it controls |
| ----------------------------------------------------------- | ---------------- |
| `helpers.SetupOptions{StartHTTPServer: true, IncludePublic: true, ServeMarketplaceUI: true, LogCapture: …}` | Real HTTP server + marketplace static assets so chromedp can bootstrap a session. |
| `httptest.NewServer` and `SetupOptions.ConfigureRoot` | Serve the local `/500.html` error-page fixture and configure `ErrorPages.InternalError` to point to it before service initialization. |
| `env.InitiateOAuth2Connection(t, connectorID, returnTo, helpers.WithActor("alice-attacker-…", root))` | Initiates the connection programmatically as the attacker — signs the request with a JWT carrying the attacker's external id. |
| `provider.Authorize(...)` (`/test/authorize`)               | Mints the OAuth code without a browser. The provider doesn't care which proxy actor owns the state — it validates against its own client/user records — so the attacker can drive this leg programmatically. |
| `env.PublicAuthUtil.GenerateBearerToken(ctx, "bob-victim-…", root, allPerms)` | Mints the JWT the victim's browser will present to the marketplace. |
| chromedp navigation to `/connectors?authToken=<victim JWT>` | Triggers the marketplace SPA's `_initiate` call, which sets the victim's `SESSION-ID` cookie. We wait on the `Connect` button as the bootstrap-complete signal. |
| chromedp navigation to the forged `/oauth2/callback?state=…&code=…` | Delivers the callback under the victim's cookie. The public service identifies the victim; state validation detects the actor mismatch and 302s to the error page. |
| `chromedp.WaitVisible("#oauth-callback-error", chromedp.ByQuery)` | Waits for the local error-page fixture to load after the 302. |
| `chromedp.Location(&finalURL)`                              | Reads the URL the browser landed on for an exact match against the configured error-page URL. |
| `logCapture.RecordsWithMessage(t, rejectionEventMessage)`   | Surfaces the structured rejection event for assertions. |
| `provider.Requests(EndpointToken, …)`                       | Confirms the token exchange path was never taken. |

## Sequence

```mermaid
sequenceDiagram
    autonumber
    participant T as Test
    participant ATK as Attacker (alice)
    participant VIC as Victim browser<br/>(chromedp)
    participant PUB as Public service<br/>(real HTTP)
    participant API as API service<br/>(real HTTP)
    participant DB as Postgres
    participant R as Redis
    participant P as OAuth provider<br/>(docker)
    participant ERR as Local error-page fixture<br/>(httptest)

    Note over T,P: Attacker leg — programmatic, runs as alice
    T->>API: POST /api/v1/connections/_initiate<br/>(JWT signed as attacker)
    API->>R: write encrypted state envelope<br/>(ActorId = attacker)
    API->>DB: persist connection (Setup, OAuth2 authorize step, attacker actor)
    API-->>T: { redirect_url containing state_id }

    T->>P: POST /test/authorize (client+user, decision=approve, state=state_id)
    P-->>T: { redirect_url with code + state_id }

    Note over T,P: Victim leg — driven by chromedp
    T->>VIC: navigate /connectors?authToken=<victim JWT>
    VIC->>PUB: GET /connectors?authToken=…
    VIC->>PUB: POST /api/v1/session/_initiate (auto by SPA)
    PUB-->>VIC: SESSION-ID cookie (scoped to victim)
    VIC->>PUB: GET /api/v1/connectors?limit=… (SPA bootstrap)
    PUB-->>VIC: render Connect button (bootstrap-complete signal)

    Note over T,P: Forged link delivery
    T->>VIC: navigate /oauth2/callback?state=…&code=…
    VIC->>PUB: GET /oauth2/callback?state=…&code=…<br/>(carries victim's SESSION-ID cookie)
    PUB->>R: GET state envelope
    PUB->>PUB: state.ActorId(attacker) ≠ caller(victim)
    PUB-->>VIC: 302 → ErrorPages.InternalError
    VIC->>ERR: GET /500.html
    ERR-->>VIC: render #oauth-callback-error
    T->>VIC: assert final URL equals configured error-page URL

    Note over T,PUB: An "oauth callback rejected" event with
    Note over T,PUB: category=actor_mismatch is in env.LogCapture.
    Note over T,P: Provider observed no /token call.
```

## Why we don't pre-create alice / bob

`NewSignedRequestForActorExternalId` signs the JWT with `claims.Actor`
populated. The auth middleware on the receiving service treats this
as a self-asserted actor and upserts on first sight, so the test does
not need to pre-create either actor row.

## Why the error page is served locally

The test starts an `httptest.NewServer` for the error page and sets
`ErrorPages.InternalError` to its `/500.html` URL through
`SetupOptions.ConfigureRoot`. This removes external DNS, TLS, availability,
and page-content dependencies from the redirect assertion. The real browser
still establishes the victim's session through the marketplace and delivers
the forged callback with that session cookie.

After the callback redirects, chromedp waits for the fixture's unique
`#oauth-callback-error` marker before reading the final URL. The exact URL
assertion verifies that the browser reached the configured error page; the
marker ensures the destination has loaded before that assertion runs.
