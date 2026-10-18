---
title: Define a connector
description: Write the YAML definition that tells AuthProxy how to authenticate to a third-party API, then publish it.
---

A connector describes one third-party provider: how it authenticates, what
the user sees in the Marketplace, and how AuthProxy should check that a
connection still works. You write it once, and every connection to that
provider uses it.

This page covers the parts of a connector you need for most providers.
[Connector setup flows](/integration/connector-setup-flow/) and [connector
predicates](/integration/connector-predicates/) cover the advanced parts.

## The shape of a connector

A connector is a YAML resource with three parts:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
metadata:
  name: github                # Unique within the namespace
  namespace: root             # Connections can be made here or in any child namespace
  labels:
    acme.com/category: developer-tools
spec:
  definition:                 # How the connector behaves (below)
    displayName: GitHub
    description: Sync issues from your GitHub repositories.
    auth:
      type: OAuth2
      # ...
  release:
    desiredState: primary     # Publish it for new connections
```

- **`metadata`** identifies the connector. The namespace controls who can see
  it. A connector in `root` is available to every tenant, while one in
  `root.tenants.acme_42` is visible only there.
- **`spec.definition`** is the behavior: display information, authentication,
  probes, and setup steps.
- **`spec.release.desiredState`** controls publication. Set it to `primary`
  to make the definition available for new connections. If you leave it out,
  the connector is saved as a `draft`, and attempts to connect to it fail
  with `connector not found`.

## Marketplace display

These fields control how the connector appears to users:

| Field | Purpose |
|---|---|
| `displayName` | The name shown to users. Required. |
| `description` | A longer explanation, shown on the connector's page. Markdown is supported. |
| `highlight` | A one-line summary for the connector list. |
| `logo` | Either `publicUrl: https://...` or `base64:` plus `mimeType:`. |
| `statusPageUrl` | The provider's status page, so users can check for outages. |
| `developerConsoleUrl`, `oauthClientUrl`, `marketplaceUrl` | Links for operators who manage your app registration with the provider. |

## Authentication

Every connector has an `auth` block whose `type` is `OAuth2`, `api-key`, or
`no-auth`.

### API keys

Use `api-key` when users paste a key or token from the provider. During setup,
AuthProxy shows a form asking for the key, encrypts it, and adds it to every
proxied request according to `placement`:

```yaml
auth:
  type: api-key
  placement:
    type: header
    headerName: X-Api-Key
```

| `placement.type` | Sends the key as | Extra fields |
|---|---|---|
| `bearer` | `Authorization: Bearer <key>` | None |
| `header` | `<headerName>: <prefix><key>` | `headerName` (required), `prefix` (optional, for example `"Token "`) |
| `query` | `?<paramName>=<key>` on the URL | `paramName` (required) |
| `basic` | `Authorization: Basic base64(<username>:<key>)` | `usernameField`: the name of the extra form field that collects the username |

### OAuth 2.0

Use `OAuth2` for providers where users sign in and grant consent. AuthProxy
handles the redirect, the code exchange, storing tokens, and refreshing them.

```yaml
auth:
  type: OAuth2
  clientId:
    envVar: GITHUB_CLIENT_ID
  clientSecret:
    envVar: GITHUB_CLIENT_SECRET
  authorization:
    endpoint: https://github.com/login/oauth/authorize
  token:
    endpoint: https://github.com/login/oauth/access_token
  scopes:
    - id: repo
      reason: Read issues from your repositories.
    - id: read:org
      reason: List the organizations you belong to.
      required: false
```

**Register the callback URL with the provider.** When you create your OAuth
app in the provider's developer console, set its redirect URI to your public
service's base URL followed by `/oauth2/callback`, for example
`https://connect.example.com/oauth2/callback`. AuthProxy sends this URL as
`redirect_uri`, and most providers reject a value that does not exactly match
what you registered.

The main fields:

- **`clientId` and `clientSecret`** are your app's credentials from the
  provider. Read them from the environment (`envVar`) or a file (`path`)
  rather than writing them inline (`value`).
- **`scopes`** lists what to request. Each scope's `reason` is shown to the
  user. A scope is required by default, which means setup fails if the
  provider does not grant it. Set `required: false` for scopes the user may
  decline.
- **`authorization.queryOverrides`** adds or replaces query parameters on the
  consent URL. For example, Google needs `prompt: consent` and
  `access_type: offline` to return a refresh token reliably.
- **`token.formOverrides`** does the same for the token request body.
- **`revocation`** gives the provider's token revocation endpoint. AuthProxy
  calls it when a connection is disconnected. `supportedTokens` can be
  `access_token`, `refresh_token`, or `both` (the default).

Less common options:

| Field | Default | Purpose |
|---|---|---|
| `grantType` | `authorization_code` | Set to `client_credentials` for machine-to-machine APIs with no user consent step. |
| `tokenEndpointAuthMethod` | `client_secret_post` | How the client secret is sent to the token endpoint. Use `client_secret_basic` if the provider requires HTTP basic auth, or `none` for public clients. |
| `token.refreshInBackground` | `true` | Refresh tokens before they expire rather than only when a request finds them expired. |
| `token.refreshTimeBeforeExpiry` | Server default | How long before expiry to refresh. |

### No authentication

Use `no-auth` for public APIs, or when you want AuthProxy's logging, rate
limiting, and request events for an API that needs no credentials:

```yaml
auth:
  type: no-auth
```

## Health checks (probes)

A probe is a request AuthProxy makes on a schedule to check that a connection
still works. If a probe fails three times in a row, the connection is marked
`unhealthy`, and the Marketplace offers the user a way to reconnect it.

```yaml
probes:
  - id: current-user
    period: 15m
    proxyHttp:
      method: GET
      url: https://api.github.com/user
```

- `proxyHttp` sends the request through the connection, with its
  credentials. This is what you want for checking that credentials are valid.
  Use `http` instead for an unauthenticated request.
- Schedule with either `period` (such as `15m`) or `cron`, not both.
- `failureThreshold` (default 3) and `recoveryThreshold` (default 1) control
  how many consecutive failures or successes change the health state.

Probes also run once during setup. A new connection does not become
`configured` until its enabled probes pass.

## Provider rate limits

When a provider responds with HTTP 429, AuthProxy reads the `Retry-After`
header and holds back further requests through that connection until the
wait is over. The `rateLimiting` block adjusts this, for example to read a
provider-specific header:

```yaml
rateLimiting:
  retryAfterHeaders: [X-RateLimit-Reset, Retry-After]
  maxRetryAfter: 10m
```

You can also set your own limits to stay under a provider's quota. See [Rate
limits](/operations/rate-limits/).

## Setup questions and conditional behavior

Some providers need extra information from the user, such as a region before
OAuth or a workspace choice after it. Add these as steps in `setupFlow`. See
[Connector setup flows](/integration/connector-setup-flow/).

Scopes, probes, and setup steps can be turned on or off per connection with
small JavaScript conditions. See [Connector
predicates](/integration/connector-predicates/).

## Apply and publish

Save the connector to a file and apply it with the CLI:

```bash
ap apply -f github.yaml
```

Applying again with changes updates the connector. Because `desiredState` is
`primary`, each changed definition is published as a new **generation**.
Existing connections stay on the generation they were created with, so a
change you publish only affects new connections until you migrate existing
ones. See [Connector generation
migrations](/operations/connector-generation-migrations/).

Other ways to manage connectors:

- Pass `--apply=<file or directory>` to `authproxy serve` to apply files at
  startup. This is how the local development stack loads its connectors.
- Use the Admin API or the
  [Terraform provider](https://github.com/rmorlok/authproxy/blob/main/terraform/provider/docs/README.md).

To remove a connector from use, see [Connector
lifecycle](/operations/connector-lifecycle/).

## Complete reference

Every field is defined in the connector JSON Schema at
[`internal/schema/resources/connectors/schema.json`](https://github.com/rmorlok/authproxy/blob/main/internal/schema/resources/connectors/schema.json).
The [development connectors](https://github.com/rmorlok/authproxy/blob/main/dev_config/default-resources.yaml)
include working examples for Google, Greenhouse, and others.
