---
title: Configuration reference
---

AuthProxy configuration is YAML validated against the canonical JSON Schema at
[`internal/schema/config/schema.json`](https://github.com/rmorlok/authproxy/blob/main/internal/schema/config/schema.json).
The development configuration demonstrates the full server shape at
[`dev_config/default.yaml`](https://github.com/rmorlok/authproxy/blob/main/dev_config/default.yaml).

## Major blocks

| Block | Purpose |
|---|---|
| `public`, `api`, `admin_api`, `worker` | Enabled services, ports, TLS, UI, and health behavior |
| `host_application`, `marketplace` | Browser login handoff and Marketplace URL |
| `system_auth` | JWT, actors, global encryption key, and DEK policy |
| `database`, `redis` | Primary database and distributed state |
| `app_metrics` | Request-event, resource-metric, and optional blob storage |
| `connectors` | Connector loading and name-based reconciliation |
| `tasks` | Task retention and worker behavior |
| `telemetry` | OTLP exporter, signals, sampling, and label projection |

Fields can use AuthProxy value sources such as direct development values,
environment variables, and file paths. Never put production credentials or
key material directly in a committed YAML file.

## Configured connector identity

Give every connector loaded from YAML a stable `name` when you omit its
immutable `id`:

```yaml
connectors:
  auto_migrate: true
  load_from_list:
    - name: google-drive
      namespace: root.integrations
      labels:
        type: google-drive
      display_name: Google Drive
      logo:
        public_url: https://example.com/google-drive.svg
      description: Connect to Google Drive.
      auth:
        type: no-auth
```

At startup, AuthProxy reconciles this entry to the live connector whose exact
name is `google-drive` in `root.integrations`. Labels are ordinary selectable
metadata and do not participate in identity. Multiple configured versions use
the same name and namespace.

An entry with an explicit `id` may omit `name`; a newly created connector then
defaults its name to the ID. To rename an existing configured connector, keep
its ID fixed and change `name`. Changing the name of an ID-less entry describes
a new connector, so the old config-managed connector enters the normal orphan
cleanup flow.

The former `connectors.identifying_labels` setting has been removed. Delete it
from existing configuration and add a stable `name` to every connector entry
that does not already specify `id`.

## Kubernetes values

The Helm chart exposes typed values for common deployment settings and merges
`config` as an advanced overlay. Its reference is
[`deploy/charts/authproxy/values.yaml`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/values.yaml)
with validation in
[`values.schema.json`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/values.schema.json).

Prefer typed chart values for database, Redis, service, ingress, Secret, and
storage configuration. Use the free-form overlay only when a server setting has
not yet been promoted into the chart schema.

## Validate changes

Start the server or render the chart in CI to exercise schema validation. For
repository changes, run:

```bash
./scripts/preflight.sh
```

Connector definitions have their own schema and authoring guides; continue
with [connector setup flows](/integration/connector-setup-flow/) and
[connector predicates](/integration/connector-predicates/).
