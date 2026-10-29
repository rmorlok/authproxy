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
| `connectors` | `archive`, `create`, `disconnect_all`, `force_state`, `get`, `list`, `list/generations`, `update` | Connector definitions, generations, metadata, and lifecycle operations |
| `keys` | `create`, `delete`, `get`, `list`, `update` | Reusable signing and encryption-key resources; `delete` also gates `DELETE /keys/{id}/unused`, which rejects namespace references and encryption history |
| `namespaces` | `create`, `get`, `list`, `update` | Namespace records, metadata, and namespace key assignments |
| `rate_limits` | `create`, `delete`, `get`, `list`, `update` | Rate-limit rules, overrides, and related evaluation endpoints |
| `request-events` | `get`, `list` | Individual and listed proxy request events |
| `secrets` | `replay` | Unredacted replay of secret-tagged fields in an otherwise authorized API response |
| `tools` | `create`, `delete`, `get`, `list`, `update` | Standalone Tool definitions under `/tools`; generated Tools are readable but changed only through their ToolSet. These verbs do not authorize Tool invocation |
| `toolsets` | `create`, `delete`, `get`, `list`, `list/generations`, `update` | ToolSet definitions and generations under `/toolsets`. `create` also gates new draft generations; `update` gates logical metadata and selector changes, draft edits, and publication. These verbs do not install, migrate, or invoke generated Tools |
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
| `list/generations` | Read or list connector or ToolSet generation data. |
| `manage` | Mutate task-queue or workflow-monitoring state. |
| `proxy` | Send a request through a connection with its credentials injected. |
| `query` | Run an aggregate application-metrics query. |
| `replay` | Return original values for fields normally redacted as secrets. |
| `schema` | Read the application-metrics schema. |

The `secrets:replay` grant does not authorize a route by itself. It only
disables response redaction after the caller passes that route's normal
permission check, so grant it sparingly. The namespace dimension is not
evaluated for `secrets:replay`, `task_monitoring`, or `workflow_monitoring`.

Tool management verbs are separate from Tool invocation. Creating or updating
a Tool needs only `tools:create` or `tools:update` in the Tool's namespace; the
bound connection must be in that same namespace, and no `connections` verb is
checked. Granting `tools:update` lets an actor change what an executable Tool
does, so treat it as trusted administration.

ToolSet management follows the same rule. `toolsets` verbs are checked in the
ToolSet's namespace, and a connection selector may only select that namespace
or its descendants. Selecting a connection grants no permission to invoke its
generated Tools, and no `tools` or `connections` verb authorizes ToolSet
management. Publishing with `toolsets:update` changes what new matching
connections install, so treat it as trusted administration as well.

See [Authentication and Authorization](/security/authentication-and-authorization/)
for namespace matching, resource ID restrictions, actor permissions, and JWT
permission intersection.
