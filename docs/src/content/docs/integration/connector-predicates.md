---
title: Connector Predicates
---

Connector predicates let a connector definition enable or require parts of a connection dynamically. They are authored in YAML and evaluated server-side for the specific connection.

Predicates currently support JavaScript only:

```yaml
if:
  javascript: |
    cfg.region === "eu" && labels["apxy/cxr/type"] === "salesforce"
```

The JavaScript must evaluate to a boolean. Syntax errors, thrown errors, `null`, `undefined`, and non-boolean results fail the operation instead of guessing the connector author's intent.

## Runtime Context

Every connector predicate receives the same variables:

| Variable | Shape | Description |
|---|---|---|
| `cfg` | object | The connection configuration collected by submitted setup steps so far. It is `{}` when no configuration has been saved yet. |
| `labels` | object | The connection labels available to the runtime, including carried-forward system labels such as `apxy/cxr/type`. |
| `annotations` | object | The connection annotations available to the runtime. It is `{}` when none are present. |

There is no `attributes` alias. Use `cfg`, `labels`, and `annotations` directly.

## Connector JavaScript Library

A connector can define top-level shared JavaScript with the `javascript` field. Use it for helper functions and constants that are reused by predicates and configure-step data-source transforms:

```yaml
javascript: |
  function isProductionConnection() {
    return labels.env === "prod";
  }

  function shouldRequestDriveActivity() {
    return cfg.sync_activity === true && isProductionConnection();
  }

auth:
  type: OAuth2
  scopes:
    - id: https://www.googleapis.com/auth/drive.activity.readonly
      if:
        javascript: shouldRequestDriveActivity()
      reason: We need activity access when activity sync is enabled.
```

AuthProxy compiles connector-level JavaScript once for the connector version, validates it by running top-level initialization without runtime variables, and then runs it in a fresh JavaScript VM for each predicate or transform evaluation. This means helpers can share code, but mutable globals do not carry state between evaluations.

Connector-level JavaScript cannot declare the reserved runtime variables `cfg`, `labels`, `annotations`, or `data`. AuthProxy injects those names for each evaluation. Syntax errors, top-level thrown errors, and reserved-name declarations fail connector validation.

The same connector-level helpers are available to setup-step predicates, OAuth scope predicates, probe predicates, and configure-step data-source transforms. Data-source transforms also receive `data`, which contains the proxied JSON response. See [Connector setup flow](/integration/connector-setup-flow/#data-sources) for transform examples.

## Setup Steps

Connector-authored setup-flow form and redirect steps support `if.javascript`. Clients only see eligible steps. See [Connector setup flow](/integration/connector-setup-flow/#conditional-steps) for setup-step examples and behavior.

Auth-method steps and AuthProxy pseudo-steps cannot be gated directly. For example, OAuth redirect steps always run when the OAuth auth method needs them. Verification is controlled indirectly by probe predicates: `apxy:verify` is inserted only when at least one probe is enabled for the connection.

## OAuth Scopes

OAuth2 scopes support two predicate-related fields:

- `if.javascript`: include or omit the scope from the effective requested scope set.
- `required.javascript`: decide whether the scope is required when AuthProxy compares requested scopes to provider-granted scopes.

```yaml
auth:
  type: OAuth2
  client_id:
    env_var: GOOGLE_CLIENT_ID
  client_secret:
    env_var: GOOGLE_CLIENT_SECRET
  authorization:
    endpoint: https://accounts.google.com/o/oauth2/v2/auth
  token:
    endpoint: https://oauth2.googleapis.com/token
  scopes:
    - id: https://www.googleapis.com/auth/drive.readonly
      reason: We need to be able to view files.

    - id: https://www.googleapis.com/auth/drive.file
      if:
        javascript: cfg.push_files === true
      reason: We need to be able to write files when file push is enabled.

    - id: https://www.googleapis.com/auth/drive.activity.readonly
      required:
        javascript: cfg.sync_activity === true
      reason: We need activity access when activity sync is enabled.
```

Scope behavior:

- A scope with no `if` is included.
- A scope whose `if.javascript` evaluates to `false` is not requested and is not part of scope-mismatch detection.
- If `if.javascript` evaluates to `false`, `required` is not evaluated for that scope.
- A scope with omitted `required` defaults to required.
- `required: false` means the scope is still requested, but AuthProxy does not fail setup if the provider omits it.
- `required.javascript` has the same semantics: when it evaluates to `false`, the scope is still requested but optional.

AuthProxy uses the same effective scope set when building the OAuth authorization URL, requesting client-credentials tokens, persisting `requested_scopes`, and checking provider-granted scopes. If a provider omits a required effective scope, AuthProxy records an OAuth scope mismatch and fails setup or token handling according to the existing OAuth error path. If a provider omits an optional effective scope, setup can continue.

### OAuth Timing

OAuth scope predicates are evaluated before OAuth authorization for authorization-code connectors. Any `cfg` values they read must already exist before OAuth begins, usually from `setup_flow.preconnect` steps. Values collected in `setup_flow.configure` happen after OAuth and are too late to affect the initial authorization URL.

For client-credentials connectors, scope predicates are evaluated before the token request. Any `cfg` values they read must be collected before that request is made.

Labels and annotations can also drive scope predicates because they are available before OAuth. Use them when the scope decision is known from connector metadata, namespace policy, or operator-provided connection annotations.

## Probes

Probes support `if.javascript` to decide whether the probe is enabled for a connection:

```yaml
probes:
  - id: drive-health
    period: 1m
    proxy_http:
      method: GET
      url: https://www.googleapis.com/drive/v3/about

  - id: calendar-list
    period: 30s
    if:
      javascript: cfg.has_calendar === true
    proxy_http:
      method: GET
      url: https://www.googleapis.com/calendar/v3/users/me/calendarList
```

Probe behavior:

- A probe with no `if` is enabled.
- A probe whose `if.javascript` evaluates to `false` is disabled for that connection.
- Disabled probes are not run during setup verification.
- Disabled periodic probes are not scheduled.
- `probe-now` enqueueing only queues enabled probes.
- Manual or stale queued tasks for a now-disabled probe skip without retry.
- Disabled probes do not block health recovery for enabled probes.
- Existing outcome rows for a now-disabled probe are still cleaned up by the outcome cleanup task using default retention thresholds.

`apxy:verify` is inserted during setup only when at least one probe is enabled for the connection. If all probes are disabled, setup advances to the next eligible configure step or completes immediately when no configure step is left.

## Versioning Notes

Adding, removing, or changing predicates or connector-level JavaScript changes the connector definition. For published connectors, publish a new connector version and migrate existing connections with the [connector version migration workflow](/operations/connector-version-migrations/).

Existing in-flight connections are evaluated against the connector version they are using. If a predicate changes in a new version, only connections migrated to that version use the new predicate behavior.
