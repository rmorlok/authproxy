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
| `public`, `api`, `adminApi`, `worker` | Enabled services, ports, TLS, UI, and health behavior |
| `hostApplication`, `marketplace` | Browser login handoff and Marketplace URL |
| `systemAuth` | JWT, actors, global encryption key, and DEK policy |
| `database`, `redis` | Primary database and distributed state |
| `appMetrics` | Request-event, resource-metric, and optional blob storage |
| `tasks` | Task retention and worker behavior |
| `telemetry` | OTLP exporter, signals, sampling, and label projection |

Fields can use AuthProxy value sources such as direct development values,
environment variables, and file paths. Never put production credentials or
key material directly in a committed YAML file.

## Connector manifests

Define connectors as `authproxy.net/v1alpha1` `Connector` resources in YAML or
JSON manifests, and manage them through
[`ap apply`](/development/cli/#apply-resource-manifests). Store manifests in
separate files or combine them in a multi-document YAML file. Server configuration
contains service and infrastructure settings; connector manifests are separate
inputs to apply.

Give each new connector a `metadata.name` and `metadata.namespace`. Include
Namespace manifests in the same apply batch when those namespaces do not exist.
See [resource manifests](/reference/resources/) for the manifest structure.

Use `ap apply -f resources.yaml` after deployment. For local development, use
[`serve --apply`](/development/cli/#apply-resources-on-server-startup) to apply
a file or directory once the API is available.

Apply identifies an existing connector by its namespace/name or `metadata.id`.
An explicit ID must already exist; omit IDs when creating new resources. For
Connector manifests, omit `metadata.generation`; apply creates a new generation
when needed. Use ConnectorGeneration manifests for explicit generation lifecycle
operations.

Removing a manifest does not delete its resource. Use explicit resource deletion
or the opt-in pruning options of `ap apply` when deletion is intended.

## Configured actor namespaces

Inline configured actors are complete `authproxy.net/v1alpha1` `Actor`
resources and set their namespace in `metadata.namespace`. For actors
discovered from public-key directories, key each source by the namespace that
owns its actors:

```yaml
systemAuth:
  actors:
    root:
      keysPath: /etc/authproxy/keys/actors/root
      permissions:
        - namespace: root.**
          resources: ["*"]
          verbs: ["*"]
    root.smoke:
      keysPath: /etc/authproxy/keys/actors/smoke
      permissions:
        - namespace: root.smoke
          resources: [connectors]
          verbs: [list]
        - namespace: root.smoke.{{external_id}}
          resources: [connections]
          verbs: [create, get, proxy]
    syncCronSchedule: "* * * * *"
```

Every `.pub` file in a source directory creates an actor in that source's
namespace. For example,
`/etc/authproxy/keys/actors/smoke/smoke-user.pub` creates `smoke-user` in
`root.smoke`. Permissions are source-specific, and permission namespaces can
use actor templates such as `{{external_id}}`. Development migration creates
each configured actor namespace and its missing parents before synchronization.
Directory sources are keyed by namespace, as shown above.

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
