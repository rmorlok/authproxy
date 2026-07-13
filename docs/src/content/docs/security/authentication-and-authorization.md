---
title: Authentication and Authorization
---

AuthProxy separates identity proof from access enforcement:

- The **host application is the identity source**. It authenticates the human
  or service, chooses a stable AuthProxy actor identity, and signs the JWT used
  for the handoff or API call.
- **AuthProxy validates and authorizes** that identity. It verifies the token or
  browser session, loads the actor, intersects any token restrictions with the
  actor's stored permissions, and validates the requested resource.

AuthProxy is not a replacement for the host application's login, MFA, account
recovery, or workforce/customer identity provider.

## Map Host Identities to Actors

An actor represents a user or service principal. Its `external_id` should be a
stable, non-reassignable identifier from the host system. Avoid mutable values
such as email addresses when a durable user id is available.

For example:

```text
Host tenant:    org_123
Host principal: usr_456
AuthProxy actor external_id: usr_456
AuthProxy resource namespace: root.tenants.org_123
```

The host application owns this mapping. AuthProxy does not infer ownership from
an email address, connection label, or other business metadata.

## Authentication Paths

| Path | Typical use | Security properties |
|---|---|---|
| Bearer JWT in `Authorization` | Host backend, CLI, automation | Signature and claims validated on each request; audience must include the receiving service |
| One-time JWT in `auth_token` | Marketplace or Admin UI session handoff | Must include expiration and nonce; nonce is atomically marked used |
| Browser session | Marketplace or Admin UI after handoff | Server-side state in Redis; encrypted session identifier in an `HttpOnly` cookie; XSRF token required for mutating session requests |
| System-signed JWT | Internal AuthProxy handoffs and OAuth state | Minted and validated by AuthProxy services; protect internal signing material as production key material |

Actor-signed tokens use an actor's private key and are verified with its
configured public key. Private keys belong in the host backend, automation
secret store, or developer keychain—not in a browser bundle, mobile client, URL,
log, or source repository.

AuthProxy validates standard JWT timing claims, its own claim structure, and
the service audience. When a token contains a nonce, it must also expire; the
nonce can establish authentication only once. A subject-only token must resolve
to an existing database actor. Integrations that include a full actor claim
should treat the signed claim as an authoritative upsert from the host identity
source.

## Browser Session Handoff

The embedded Marketplace and Admin UI use a delegated session flow:

1. The user authenticates with the host application.
2. The host maps the user to an actor and decides what they may access.
3. The host backend signs a short-lived JWT with the target AuthProxy service
   in `aud`, an expiration, and a one-time nonce.
4. The host redirects to the AuthProxy UI with `auth_token=<jwt>`.
5. The UI calls `POST /api/v1/session/_initiate`; AuthProxy validates the token, marks
   the nonce used, and establishes a server-side session.
6. Later UI requests use the session cookie and an XSRF token for mutations.

Keep the handoff safe:

- Generate tokens only after host authentication and authorization.
- Keep the signing key in the backend and limit who can invoke the signing
  operation.
- Use the shortest practical expiration and a fresh nonce for every handoff.
- Allowlist `return_to` destinations. Do not turn the login endpoint into an
  open redirect.
- Avoid logging URLs that contain `auth_token`; scrub query strings at proxies,
  analytics tools, and error trackers.
- Configure the external HTTPS URL correctly. Session cookies are `Secure` when
  AuthProxy is configured as HTTPS, so that setting must match ingress or proxy
  TLS termination.
- Limit cookie domains and CORS origins. Choose `SameSite` deliberately for the
  embedding and OAuth callback topology.

Session state is stored in Redis and expires with the configured session
timeout. Protect Redis with network isolation, authentication, TLS where
available, persistence/backup policy appropriate to the deployment, and a
process for invalidating sessions during an incident or user offboarding.

## Permission Model

A permission grants a verb on a resource type within a namespace and can
optionally restrict specific resource ids:

```yaml
permissions:
  - namespace: root.tenants.org_123.**
    resources: [connectors, connections]
    verbs: [get, list, create, update]
  - namespace: root.tenants.org_123.**
    resources: [connections]
    resource_ids: [cxn_example]
    verbs: [proxy]
```

The dimensions are:

- `namespace`: an exact path or a `**` subtree matcher such as
  `root.tenants.org_123.**`;
- `resources`: API resource types, with `*` as a wildcard;
- `verbs`: operations such as `get`, `list`, `create`, `update`, or `proxy`,
  with `*` as a wildcard; and
- `resource_ids`: an optional restriction to named resources.

Actor permissions are additive: any actor permission that matches can allow an
operation. If a JWT also carries permissions, those permissions are
restrictions. The action must be allowed by both the actor's stored permissions
and the token's permission set. A caller cannot add a broad token permission to
expand a narrow actor grant.

Routes validate the namespace and, where relevant, the loaded resource id.
List and aggregate routes constrain their database queries to effective
namespace matchers rather than relying on a client-supplied filter.

## Least-Privilege Tokens

Use token restrictions when a user delegates a narrow operation to automation
or an agent. An actor might be able to manage every connection in a tenant,
while a task token can be limited to proxying through one connection:

```yaml
permissions:
  - namespace: root.tenants.org_123
    resources: [connections]
    resource_ids: [cxn_salesforce]
    verbs: [proxy]
```

Also set a short expiration and only the service audiences the task needs.
Token restrictions reduce blast radius; they do not replace actor
deprovisioning or signing-key rotation.

## Labels and Permission Templates

**Labels and annotations are metadata, not authorization.** They support
mapping, selection, telemetry dimensions, and reporting. A resource label or
label selector does not grant access and must not be used as an ACL.

Permission namespaces can explicitly template trusted actor data:

```yaml
permissions:
  - namespace: root.tenants.{{labels.tenant_id}}.**
    resources: [connections]
    verbs: [get, list, create, proxy]
```

Here the permission is the authorization rule. The actor label only supplies a
value to that rule. If you use this pattern, the host identity system must own
and validate the referenced actor label; do not let an end user choose it. A
missing or invalid template value does not match.

Prefer direct, explicit namespace grants when they are practical. See
[Labels and Annotations](/concepts/labels-and-annotations/) for metadata
semantics.

## Service Exposure

AuthProxy separates its HTTP surfaces so they can have different network
policies:

- `public` handles UI sessions, Marketplace APIs, and OAuth callbacks that may
  require internet access;
- `api` handles application integration and proxy requests;
- `admin-api` manages actors, connectors, connections, keys, and operations;
  and
- `worker` processes background work and normally exposes only health checks.

Keep `api` and especially `admin-api` private unless the deployment has a
specific, reviewed reason to expose them. Public-service connection proxying is
disabled by default; enable it only when browser/session clients need to call a
third party directly through AuthProxy.

TLS, certificates, reverse-proxy configuration, network policy, firewall
rules, service-to-service authentication, and datastore transport security are
operator responsibilities. AuthProxy can terminate TLS itself, but a Helm or
Kustomize deployment commonly terminates TLS at ingress. In either case, ensure
the configured external URLs, HTTPS state, cookie flags, and allowed origins
describe the real topology.

## Review Guidance

Before adding an actor, route, or integration:

1. Identify the stable host principal and tenant namespace.
2. List the exact resources and verbs it needs; avoid wildcard grants.
3. Decide whether a token should further restrict resource ids or namespaces.
4. Verify the receiving service audience, expiration, and nonce behavior.
5. Test both a permitted resource and a resource in a sibling namespace.
6. Test list/query endpoints to ensure results are namespace-constrained.
7. Confirm labels are used only for metadata or as controlled input to an
   explicit permission template.
8. Define offboarding: stop host authentication, remove grants, rotate or
   revoke signing credentials, invalidate sessions, and disconnect or revoke
   third-party credentials where appropriate.
