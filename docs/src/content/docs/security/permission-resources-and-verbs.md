---
title: Permission Resources and Verbs
description: Reference every resource type and verb available in AuthProxy permissions.
---

Use this reference when granting actor permissions or adding request-level JWT
restrictions. Resource and verb strings are exact and case-sensitive. Although
the permission schema accepts any non-empty string, only the combinations that
AuthProxy checks have an effect.

`*` is valid in either `resources` or `verbs` and matches every value in that
dimension. Prefer the explicit values below for least-privilege grants.

## Resource and Verb Matrix

| Resource type | Available verbs | Controls |
|---|---|---|
| `actors` | `create`, `delete`, `get`, `list`, `update` | Actor records, permissions, labels, annotations, and signing keys |
| `app-metrics` | `query`, `schema` | Aggregate application-metric queries and metric-schema discovery |
| `connections` | `create`, `disconnect`, `force_state`, `get`, `list`, `proxy`, `update` | Connection setup, configuration, lifecycle, and authenticated proxy requests |
| `connectors` | `archive`, `create`, `disconnect_all`, `force_state`, `get`, `list`, `list/versions`, `update` | Connector definitions, versions, metadata, and lifecycle operations |
| `keys` | `create`, `delete`, `get`, `list`, `update` | Reusable signing and encryption-key resources |
| `namespaces` | `create`, `get`, `list`, `update` | Namespace records, metadata, and namespace key assignments |
| `rate_limits` | `create`, `delete`, `get`, `list`, `update` | Rate-limit rules, overrides, and related evaluation endpoints |
| `request-events` | `get`, `list` | Individual and listed proxy request events |
| `secrets` | `replay` | Unredacted replay of secret-tagged fields in an otherwise authorized API response |
| `task_monitoring` | `get`, `list`, `manage` | Asynq queue, server, scheduler, and task inspection or mutation |
| `workflow_monitoring` | `get`, `list`, `manage` | Workflow instance inspection, cancellation, and removal |

## Specialized Verbs

Most resources use the conventional `create`, `get`, `list`, `update`, and
`delete` verbs. The specialized verbs mean:

| Verb | Meaning |
|---|---|
| `archive` | Archive a connector. |
| `disconnect` | Disconnect one connection. |
| `disconnect_all` | Disconnect all connections for a connector. |
| `force_state` | Force a connector or connection into a lifecycle state. |
| `list/versions` | Read or list connector-version data. |
| `manage` | Mutate task-queue or workflow-monitoring state. |
| `proxy` | Send a request through a connection with its credentials injected. |
| `query` | Run an aggregate application-metrics query. |
| `replay` | Return original values for fields normally redacted as secrets. |
| `schema` | Read the application-metrics schema. |

The `secrets:replay` grant does not authorize a route by itself. It only
disables response redaction after the caller passes that route's normal
permission check, so grant it sparingly. The namespace dimension is not
evaluated for `secrets:replay`, `task_monitoring`, or `workflow_monitoring`.

See [Authentication and Authorization](/security/authentication-and-authorization/)
for namespace matching, resource ID restrictions, actor permissions, and JWT
permission intersection.
