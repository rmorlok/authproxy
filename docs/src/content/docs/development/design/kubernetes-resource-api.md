---
title: Kubernetes-style resource API migration
description: Proposed contract and migration inventory for moving AuthProxy API v1 resources to authproxy.net/v1alpha1 objects.
pagefind: false
banner:
  content: Proposal only — this contract is being implemented on the codex/k8s-resource-api feature branch and is not current product behavior.
---

This decision defines the target wire contract and the complete migration
inventory for [#829](https://github.com/rmorlok/authproxy/issues/829). It is the
gate for implementation work: a surface listed here must either adopt the
resource contract or remain outside it for an explicit reason.

The inventory was taken from `main` at commit `0ce5d49c` on August 16, 2026. It
covers all routes emitted by the server route lister, the root configuration
schema, JWT and one-time token claims, API schema types, registered Asynq tasks
and workflows, generated contracts, and first-party consumers. A route or
serialized type added while the feature branch is active must be added to this
inventory.

## Status and delivery model

- Status: accepted for implementation; not yet shipped.
- Tracking issue: [#829](https://github.com/rmorlok/authproxy/issues/829).
- Feature branch: `codex/k8s-resource-api`.
- Child PRs target the feature branch, not `main`.
- The final integration PR targets `main` and must not merge until every item in
  this inventory is migrated or retained as a documented exception.

The HTTP route prefix stays `/api/v1`. This is a breaking change to API v1, not
the introduction of an HTTP v2. AuthProxy is not deployed to production, so the
project will not carry compatibility handlers, legacy request decoders,
dual-read or dual-write logic, or deprecation endpoints.

Existing data-bearing demo and development environments must be destroyed and
recreated when this work lands. In particular, old encrypted connector
definitions are unsupported. No background task, resumable migration, or
legacy decoder will rewrite them. The existing encrypted column can store the
new definition representation, so the shape change alone does not require a SQL
schema migration.

## Decision

### Version and kind

Every AuthProxy resource uses:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
```

`apiVersion` versions the schema independently from the `/api/v1` HTTP routing
namespace. Kind names are singular PascalCase. A list uses `<Kind>List`. A
versioned connector definition is still `kind: Connector`; its definition
version is `metadata.generation`.

### Resource envelope

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Example
metadata:
  id: ex_01JXYZ...
  name: example
  namespace: root/acme
  generation: 1
  labels: {}
  annotations: {}
  createdAt: 2026-08-16T12:00:00Z
  updatedAt: 2026-08-16T12:00:00Z
spec: {}
status: {}
```

Fields are included only where the resource supports them:

- `metadata.id` is the durable AuthProxy identifier. It is normally
  server-owned, though configuration reconciliation may accept an explicit ID.
- `metadata.name` is the human-stable resource name.
- `metadata.namespace` is the owning namespace path. On a Namespace object it
  is the parent path; the namespace's canonical path is derived from parent and
  name. Root has no parent.
- `metadata.generation` identifies a resource generation when AuthProxy has
  meaningful generations. Connector definition versions use it.
- labels, annotations, and timestamps have one representation in ObjectMeta.
- `spec` contains desired configuration.
- `status` contains server-observed state and is rejected on client writes.
  It is omitted when the resource has no useful observed state.

Create and patch policies decide which metadata fields a caller may provide.
Response construction must not reuse request validation accidentally.

### Lists, references, and actions

Resource list responses use a Kubernetes-style list object:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectorList
metadata:
  continue: ""
  remainingItemCount: 0
items: []
```

References use a shared ObjectReference containing the minimum stable identity:
`apiVersion`, `kind`, and the applicable `id`, `name`, `namespace`, and
`generation`. References do not embed another resource's spec or status.

Imperative operations are typed action objects. They use TypeMeta plus a target
reference in metadata and action input in spec; an asynchronous or otherwise
observable result may use status. They are not durable managed resources. Kind
names follow `<Resource><Action>`, such as `ConnectionDisconnect`.

Analytical results and protocol payloads do not become resources merely to gain
an envelope. The inventory below identifies those exceptions.

### Shared schema ownership

[#831](https://github.com/rmorlok/authproxy/issues/831) implements the shared
contract once rather than repeating it in every resource package:

- `internal/schema/resources/meta` owns TypeMeta, ObjectMeta, ObjectReference,
  Condition, the API-version primitive, and common metadata validation.
- `internal/schema/api/v1alpha1` owns ListMeta, resource-list and action
  transport composition, and version-specific API helpers. This keeps API
  envelopes and pagination out of the resource-definition tree.
- `internal/schema/manifest` owns strict JSON/YAML GVK dispatch and any required
  multi-document decoding.
- Each resource package owns its kind constant, Spec, Status, resource object,
  lifecycle validation, and resource-specific defaulting.
- `internal/schema/resources/actor` and
  `internal/schema/resources/connection` become the homes for resource
  contracts currently split between auth/core/API packages.
- `internal/schema/resources/connectors` becomes provider-definition schema and
  validation only; logical connector identity and generation belong to the
  Connector resource type.
- Existing namespace, key, and rate-limit resource packages adopt the common
  metadata contract without importing `internal/schema/api`.
- `internal/schema/api` changes the existing v1 transport directly and composes
  resource-owned contracts. Resource packages never import it.

Shared helpers cover TypeMeta validation, source- and operation-aware
defaulting, `ValidateFor(create|update|response)`, immutable and server-owned
metadata checks, metadata patch application with explicit nil-versus-empty
semantics, deterministic spec canonicalization and hashing, and constructors
for lists, references, and conditions. Database rows remain flat; conversion at
core/database boundaries prevents persistence models from becoming public wire
contracts. Authorization and business side effects remain in core and routes.

### Connector generations

A logical connector keeps one `metadata.id`, `metadata.name`, and namespace.
Each definition version is returned as another Connector object with a distinct
`metadata.generation`:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
metadata:
  id: con_01JXYZ...
  name: greenhouse
  namespace: root
  generation: 3
  labels:
    environment: demo
  createdAt: 2026-08-16T12:00:00Z
  updatedAt: 2026-08-16T12:05:00Z
spec:
  release:
    desiredState: primary
  definition:
    displayName: Greenhouse
    description: Connect AuthProxy to Greenhouse.
    auth:
      type: api-key
      placement:
        type: bearer
status:
  state: primary
  semanticHash: 0ad98e1
```

Only canonical `spec.definition` is stored in the encrypted connector
definition column. Identity, namespace, generation, release state, labels,
annotations, timestamps, and status are not duplicated there. The semantic hash
also covers canonical `spec.definition` only. Consequently a metadata or
release-state change does not manufacture a connector generation.

Configuration may omit ID and generation to reconcile by namespace/name plus
the semantic hash. When it supplies both `metadata.id` and
`metadata.generation`, the existing explicit sequential-generation rules apply.

## Registered route inventory

The tables use `GET|POST` to mean that both methods are registered. A
`{labels,annotations}` family expands to both path segments; its `:key` route
expands to `:label` or `:annotation` as it exists today. Those expansions account
for every concrete registration in the audited route set.

### Service exposure

| Surface | Admin API | API | Public | Worker | Owner |
| --- | --- | --- | --- | --- | --- |
| Connector | yes | yes | marketplace mode | no | #832, #839 |
| Connection and setup | yes | yes | marketplace mode | no | #840 |
| Namespace | yes | yes | no | no | #834 |
| Key | yes | no | no | no | #835 |
| RateLimit | yes | yes | no | no | #836 |
| Actor | yes | yes | no | no | #837, #838 |
| Request events and metrics | yes | yes | no | no | #842 |
| Notification | yes | yes | marketplace mode | no | #841 |
| Resource search | yes | no | no | no | #841 |
| Task resolution | no | yes | marketplace mode | no | #843 |
| Task/workflow monitoring | yes | no | no | no | #843 |
| Session | UI mode | no | marketplace mode | no | #838, #848 |
| Connection proxy | no | yes | proxy mode | no | #840 |
| OAuth/setup browser protocol | no | no | yes | no | #840 |
| Ping/health | yes | yes | yes | yes | exception |
| Swagger | yes | yes | no | no | #833, #849 |
| Static SPA/error pages | UI mode | no | UI mode | no | exception |

### Managed resource routes

| Resource | Registered method and path | Target |
| --- | --- | --- |
| Connector | `GET|POST /api/v1/connectors` | `ConnectorList` or `Connector` |
| Connector | `GET|PATCH /api/v1/connectors/:id` | `Connector`; patch metadata/spec |
| Connector generation | `GET|POST /api/v1/connectors/:id/versions` | `ConnectorList` or `Connector`; retain URL but use `metadata.generation` |
| Connector generation | `GET|PATCH /api/v1/connectors/:id/versions/:version` | `Connector`; route version maps to generation |
| Connector action | `POST /api/v1/connectors/:id/_disconnectAll` | `ConnectorDisconnectAll` action; task/workflow result in status |
| Connector action | `POST /api/v1/connectors/:id/_archive` | `ConnectorArchive` action; result references Connector |
| Connector action | `PUT /api/v1/connectors/:id/versions/:version/_forceState` | `ConnectorForceState` action; response Connector |
| Connector metadata | `GET /api/v1/connectors/:id/{labels,annotations}` and `GET|PUT|DELETE /api/v1/connectors/:id/{labels,annotations}/:key` | retire; use `PATCH` on Connector metadata |
| Connector-generation metadata | `GET /api/v1/connectors/:id/versions/:version/{labels,annotations}` and `GET|PUT|DELETE /api/v1/connectors/:id/versions/:version/{labels,annotations}/:key` | retire; use `PATCH` on generation metadata |
| Connection | `POST /api/v1/connections/_initiate` | `ConnectionInitiate` action; response Connection plus setup status |
| Connection | `GET /api/v1/connections` | `ConnectionList` |
| Connection | `GET|PATCH /api/v1/connections/:id` | `Connection`; patch metadata/spec |
| Connection setup | `POST /api/v1/connections/:id/_submit` | `ConnectionSubmit` action |
| Connection setup | `GET /api/v1/connections/:id/_setupStep` | `ConnectionSetupStep` read projection |
| Connection setup | `GET /api/v1/connections/:id/_dataSource/:sourceId` | `DataSourceOptionList` projection |
| Connection action | `POST /api/v1/connections/:id/_disconnect` | `ConnectionDisconnect` action |
| Connection action | `POST /api/v1/connections/:id/_abort` | `ConnectionAbort` action |
| Connection action | `POST /api/v1/connections/:id/_reconfigure` | `ConnectionReconfigure` action |
| Connection action | `POST /api/v1/connections/:id/_migrateVersion` | `ConnectionMigrateGeneration` action |
| Connection action | `POST /api/v1/connections/:id/_cancelSetup` | `ConnectionCancelSetup` action |
| Connection action | `POST /api/v1/connections/:id/_retry` | `ConnectionRetry` action |
| Connection action | `POST /api/v1/connections/:id/_reauth` | `ConnectionReauthenticate` action |
| Connection action | `PUT /api/v1/connections/:id/_forceState` | `ConnectionForceState` action; response Connection |
| Connection metadata | `GET /api/v1/connections/:id/{labels,annotations}` and `GET|PUT|DELETE /api/v1/connections/:id/{labels,annotations}/:key` | retire; use `PATCH` on Connection metadata |
| Connection scopes | `GET /api/v1/connections/:id/scopes` | `ConnectionScopeList` projection |
| Namespace | `GET|POST /api/v1/namespaces` | `NamespaceList` or `Namespace` |
| Namespace | `GET|PATCH /api/v1/namespaces/:path` | `Namespace`; `:path` remains lookup identity |
| Namespace metadata | `GET /api/v1/namespaces/:path/{labels,annotations}` and `GET|PUT|DELETE /api/v1/namespaces/:path/{labels,annotations}/:key` | retire; use `PATCH` on Namespace metadata |
| Namespace key | `GET|PUT|DELETE /api/v1/namespaces/:path/key` | retire; read or patch `Namespace.spec.keyRef` |
| Key | `GET|POST /api/v1/keys` | `KeyList` or `Key` |
| Key | `GET|PATCH|DELETE /api/v1/keys/:id` | `Key`; secret spec is write-only/redacted |
| Key metadata | `GET /api/v1/keys/:id/{labels,annotations}` and `GET|PUT|DELETE /api/v1/keys/:id/{labels,annotations}/:key` | retire; use `PATCH` on Key metadata |
| RateLimit | `GET|POST /api/v1/rate-limits` | `RateLimitList` or `RateLimit` |
| RateLimit | `GET|PATCH|DELETE /api/v1/rate-limits/:id` | `RateLimit` |
| RateLimit action | `POST /api/v1/rate-limits/_dryRun` | `RateLimitDryRun` action; matches and failures in status |
| RateLimit metadata | `GET /api/v1/rate-limits/:id/{labels,annotations}` and `GET|PUT|DELETE /api/v1/rate-limits/:id/{labels,annotations}/:key` | retire; use `PATCH` on RateLimit metadata |
| Actor | `GET|POST /api/v1/actors` | `ActorList` or `Actor` |
| Actor | `GET|PATCH|DELETE /api/v1/actors/:id` | `Actor` |
| Actor alternate lookup | `GET|PATCH|DELETE /api/v1/actors/external-id/:externalId` | same Actor contract; external ID remains a lookup key |
| Actor metadata | `GET /api/v1/actors/:id/{labels,annotations}` and `GET|PUT|DELETE /api/v1/actors/:id/{labels,annotations}/:key` | retire; use `PATCH` on Actor metadata |

Retiring the metadata and namespace-key subroutes is intentional. The breaking
contract should have one patch/default/validation path for ObjectMeta and Spec,
not preserve a parallel key-value API that recreates the duplication this
project is removing.

### Read models, analytical APIs, and typed actions

| Surface | Registered method and path | Classification and target | Owner |
| --- | --- | --- | --- |
| Notification | `GET /api/v1/notifications` | read-only `NotificationList`; each Notification has identity/content in metadata/spec and actor-specific viewed/actionability in status | #841 |
| Notification view | `POST /api/v1/notifications/_viewed`; `POST /api/v1/notifications/:id/_viewed` | `NotificationView` action, accepting one or more Notification references | #841 |
| Search | `GET /api/v1/search/resources` | `ResourceSearchResultList` projection; items contain ObjectReference plus bounded summary/match fields, not copied resource specs | #841 |
| Request event | `GET /api/v1/metrics/request-events`; `GET /api/v1/metrics/request-events/:id` | immutable read-only `RequestEventList`/`RequestEvent` | #842 |
| Metrics query | `POST /api/v1/metrics/query` | analytical `MetricsQuery`/`MetricsQueryResult`; typed but not a managed resource | #842 |
| Metrics schema | `GET /api/v1/metrics/schema` | `MetricsSchema` projection; typed but not a managed resource | #842 |
| Task resolution | `GET /api/v1/tasks/:encryptedTaskInfo` | read-only `Task` operational projection over Asynq or workflow state | #843 |
| Task queues | `GET /api/v1/task-monitoring/queues`; `GET /api/v1/task-monitoring/queues/:queue`; `GET /api/v1/task-monitoring/queues/:queue/history` | `TaskQueueList`, `TaskQueue`, and `TaskQueueHistory` operational projections | #843 |
| Tasks | `GET /api/v1/task-monitoring/queues/:queue/tasks/:state`; `GET /api/v1/task-monitoring/queues/:queue/tasks/:state/:taskId` | `TaskList`/`Task` operational projections | #843 |
| Task workers/schedule | `GET /api/v1/task-monitoring/servers`; `GET /api/v1/task-monitoring/scheduler-entries` | `TaskServerList` and `SchedulerEntryList` projections | #843 |
| Task actions | `POST /api/v1/task-monitoring/queues/:queue/tasks/:taskId/_run`; `POST /api/v1/task-monitoring/queues/:queue/tasks/:taskId/_archive`; `POST /api/v1/task-monitoring/queues/:queue/tasks/:taskId/_cancel`; `DELETE /api/v1/task-monitoring/queues/:queue/tasks/:taskId` | `TaskRun`, `TaskArchive`, `TaskCancel`, and `TaskDelete` actions | #843 |
| Queue actions | `POST /api/v1/task-monitoring/queues/:queue/_pause`; `POST /api/v1/task-monitoring/queues/:queue/_unpause`; `POST /api/v1/task-monitoring/queues/:queue/archived/_runAll`; `POST /api/v1/task-monitoring/queues/:queue/retry/_runAll`; `DELETE /api/v1/task-monitoring/queues/:queue/archived`; `DELETE /api/v1/task-monitoring/queues/:queue/completed` | typed queue/bulk actions with affected-count status | #843 |
| Workflows | `GET /api/v1/workflow-monitoring/instances`; `GET /api/v1/workflow-monitoring/instances/:instanceId/:executionId` | `WorkflowInstanceList`/`WorkflowInstance` operational projections | #843 |
| Workflow history/tree | `GET /api/v1/workflow-monitoring/instances/:instanceId/:executionId/history`; `GET /api/v1/workflow-monitoring/instances/:instanceId/:executionId/tree` | `WorkflowHistoryEventList` and `WorkflowInstanceTree` projections | #843 |
| Workflow actions | `POST /api/v1/workflow-monitoring/instances/:instanceId/:executionId/_cancel`; `DELETE /api/v1/workflow-monitoring/instances/:instanceId/:executionId` | `WorkflowCancel` and `WorkflowDelete` actions | #843 |

The exact read-model fields are implemented by the owning issues, but their
classification is fixed here: they may use TypeMeta/kind for discriminated SDK
types without pretending that Asynq queues, metric series, or search hits are
declarative resources stored by AuthProxy.

### Protocol and infrastructure exceptions

| Surface | Registered method and path | Reason it remains unwrapped | Owner |
| --- | --- | --- | --- |
| Structured proxy | `POST /api/v1/connections/:id/_proxy` | request/response represents arbitrary third-party HTTP traffic | #840 |
| Raw proxy | all methods on `/api/v1/connections/:id/_proxyRaw` | streams third-party method, headers, status, and body without buffering or an AuthProxy envelope | #840 |
| Session | `POST /api/v1/session/_initiate`; `POST /api/v1/session/_terminate` | authentication/cookie protocol; typed parameters and errors remain protocol objects | #838, #848 |
| OAuth | `GET /oauth2/redirect`; `GET /oauth2/callback` | browser and provider redirect protocol with opaque provider query parameters | #840 |
| Redirect setup | `GET|POST /setup/connections/:id/advance`; `GET|POST /setup/connections/:id/abort` | browser redirect/form protocol backed by a one-time token | #840 |
| Error page | `POST /error` | browser HTML error rendering | #848 |
| Health | `GET /ping`; `GET /healthz` on HTTP services and worker health server | infrastructure probes must retain simple status semantics | exception |
| Swagger | `GET /swagger`; `GET /swagger/*any` on API and Admin API | generated documentation assets | #833, #849 |
| Static UI | Admin and Marketplace SPA mounts/fallbacks | static files and client-side routes, not API objects | #845, #846 |

There are no AuthProxy product webhook routes or serialized webhook delivery
payloads in the audited revision. The `webhooks:full` text in demo configuration
is an OAuth provider scope literal. If webhook support is added before this
migration finishes, its route and payload classification must be added here.

## Current DTO-to-target inventory

This table catches serialized types that are easier to miss than their route.
OpenAPI-only adapters follow the same target as the runtime type and should
disappear where the new resource structs are directly representable.

| Current contract family | Target | Owner |
| --- | --- | --- |
| `ConnectorJson`, `ConnectorVersionJson`, create/update/version request types | Connector/ConnectorList using generation; create/patch policies over the same resource contract | #832, #839 |
| connector lifecycle and force-state request/response types | typed Connector actions and Connector/Task references in results | #839, #843 |
| `ConnectionJson`, list/update types | Connection/ConnectionList | #840 |
| initiate/submit/setup redirect/form/verifying/complete/error variants and data-source options | typed connection actions and setup/read projections | #840 |
| disconnect, migrate-version, retry, reauth, force-state types | typed Connection actions; `targetVersion` becomes connector generation reference | #840 |
| `NamespaceJson`, create/update/list types | Namespace/NamespaceList | #834 |
| `NamespaceKeyJson`, `SetNamespaceKeyRequestJson` | `Namespace.spec.keyRef`; no separate subresource DTO | #834, #835 |
| `KeyJson`, create/update/list types | Key/KeyList with write-only/redacted secret spec | #835 |
| `RateLimitJson`, create/update/list types | RateLimit/RateLimitList | #836 |
| rate-limit dry-run and synthetic proxy request types | RateLimitDryRun action and analytical status | #836 |
| `ActorJson`, create/update/list types | Actor/ActorList | #837 |
| `NotificationJson`, upsert/list/view types | read-only Notification resources, internal write model, and NotificationView action | #841 |
| `SearchResourceSummaryJson`, match and response types | ObjectReference-based ResourceSearchResultList projection | #841 |
| `RequestEventJson` and list types | immutable RequestEvent/RequestEventList | #842 |
| metrics query/range/series/schema types | typed analytical query/result/schema contracts, not managed resources | #842 |
| task queue/server/scheduler/task/bulk result DTOs | operational projections and typed actions | #843 |
| workflow instance/history/tree/list DTOs | operational projections and typed actions | #843 |
| `KeyValueJson`, `PutKeyValueRequestJson` | removed with label/annotation subroutes; ObjectMeta patching is authoritative | #833 plus each resource issue |
| `ErrorResponse` | common API error contract; not a resource | #833 |
| session initiate types | auth protocol contract; Actor inside JWT follows the resource policy | #838 |
| `ProxyResponseJson` and raw proxy types | third-party protocol payloads; no resource envelope | #840 |

## Configuration inventory

The root configuration file remains an operational configuration document. It
does not itself become an AuthProxy resource list. Only entries that declare
managed resources use manifests.

| Configuration surface | Target | Owner |
| --- | --- | --- |
| `connectors.loadFromList` | list of complete Connector manifests; connector loader settings such as `autoMigrationLockDuration` remain ordinary operational config | #832, #839 |
| connector definition auth, scopes, probes, setup flow, migrations, telemetry, and rate-limiting blocks | provider-only schema under `Connector.spec.definition` | #832, #839 |
| `systemAuth.actors` inline list | restricted Actor manifests; config normalization applies the config source policy | #837, #838 |
| `ConfiguredActorsExternalSource` | remains a source descriptor; discovered entries normalize into Actor manifests before reconciliation | #837, #838 |
| `AdminUser`/`AdminUsers` schema types | currently not wired into Root or a service; remove as dead schema or normalize through the same Actor policy before reuse | #838 |
| `systemAuth.jwtSigningKey`, `globalAesKey`, actor/admin public keys, TLS keys, and storage-provider `KeyData` | cryptographic provider configuration, not durable Key resources; use ObjectReference only where they refer to a managed Key | #835, #838 |
| database, Redis, blob storage, logging, telemetry, app metrics, OAuth timing, services, CORS, cookies, tasks, marketplace, host application, error pages, and dev settings | remain operational config, not resource manifests | explicit exception |
| `connections.setupTtl` | remains operational lifecycle configuration; it does not declare Connection resources | #840 |
| `dev_config`, deployment values/templates, demos, and integration fixtures | update every embedded connector/actor manifest and remove old examples | #848 |
| configuration JSON Schema | regenerate for resource entries and retain operational blocks | #831, #839, #848, #849 |

No configuration compatibility decoder will accept both flat and resource
forms. Unknown GVKs and old connector/actor shapes fail strict decoding.

## Authentication and token inventory

| Serialized payload | Target policy | Owner |
| --- | --- | --- |
| `jwt.AuthProxyClaims` | registered JWT claims remain top-level; `actor` becomes a restricted `authproxy.net/v1alpha1` Actor object | #838 |
| JWT Actor | require `sub == actor.spec.externalId`; namespace claim equals `actor.metadata.namespace`; forbid metadata ID/timestamps/status/signing material; token permissions may only narrow Actor spec permissions | #838 |
| session JWT/cookie authentication | use the same AuthProxyClaims and restricted Actor contract; no second actor representation | #838 |
| OAuth encrypted Redis state | internal versioned protocol payload; use connector generation and stable resource references internally, not a public resource envelope | #839, #840 |
| setup-token encrypted Redis `Claims` | internal one-time protocol payload; retain connection/actor binding and update reference naming where resource changes require it | #840 |
| encrypted `tasks.TaskInfo` URL token | internal locator for Asynq/workflow state; expose the resolved Task projection, not the locator payload | #843 |

Old flat JWT Actor claims are rejected after the breaking change. There is no
dual claim decoder.

## Internal event, task, and workflow inventory

Internal queue/workflow payloads are persistence protocols, not API resources.
They stay compact and explicitly versioned. If they embed resource identity,
they adopt ID/reference/generation terminology and receive a payload version
when the stored shape changes. Fresh environments mean no compatibility worker
is required, but versioned names prevent ambiguity in new persisted work.

### Asynq tasks

| Task type | Current payload | Target treatment |
| --- | --- | --- |
| `auth_tasks:clear_expired_nonces` | none | unchanged internal task |
| `auth_tasks:sync_external_source` | none | actor loader produces Actor resources; #838 |
| `oauth2:refresh_expiring_oauth_tokens` | none | unchanged scheduler |
| `oauth2:refresh_oauth_token` | connection ID | use stable Connection reference terminology; #840, #843 |
| `database:purge_soft_deleted` | none | unchanged internal task |
| `database:cleanup_stale_connections` | none | unchanged internal task |
| `database:propagate_namespace_labels` | namespace path | update for canonical Namespace reference/path semantics; #834, #843 |
| `database:propagate_connector_labels` | connector ID | update for Connector reference/generation rules; #839, #843 |
| `database:reconcile_carry_forward_labels` | none | consume common metadata helpers; #831, #843 |
| `core:migrate_connections_between_connector_versions` | none | rename version concepts to generation in behavior and monitoring; #839, #843 |
| `core:probe` | connection ID and probe ID | internal task with Connection reference terminology; #840, #843 |
| `core:verify_connection` | connection ID | internal task with Connection reference terminology; #840, #843 |
| `core:probe_outcome_cleanup` | retention seconds | unchanged internal task |
| `encrypt:reencrypt_all` | none | unchanged; it is key rotation, not connector-format conversion |
| `encrypt:generate_data_encryption_keys` | none | consumes Key resources through core/database boundaries; #835, #843 |
| `encrypt:sync_keys_to_database` | none | consumes Key resources through core/database boundaries; #835, #843 |
| `app_metrics:resource_snapshot` | none | snapshot projections adopt resource metadata at read boundary; #842, #843 |

No task may be added to convert legacy encrypted connector definitions.

### Workflows

| Workflow | Current input | Target treatment |
| --- | --- | --- |
| `core.connection.disconnect.v1` | connection ID, timeout | internal v1 payload with stable Connection reference terminology |
| `core.connector.disconnect_connections.v1` | connector ID, timeout | internal v1 payload; Connector ID remains logical identity |
| `core.connector.archive.v1` | connector ID, timeout | internal v1 payload; API action resolves to this workflow |
| `core.connection.migrate_version.v1` | connection ID, target version, timeout | introduce a new workflow payload/name using target connector generation rather than silently changing persisted v1 semantics |

Workflow monitoring history attributes are backend-owned arbitrary data. They
remain a projection and must be redacted/bounded; they are not decoded as
AuthProxy resources.

### Events and projections

| Producer/data | Classification | Owner |
| --- | --- | --- |
| app-metrics request log / `RequestEventJson` | immutable RequestEvent read resource with typed references; sensitive request/response fields remain redacted | #842 |
| app-metrics resource snapshots and metric series | analytical storage/projection, not resources | #842 |
| core notification records and migration-hook notification definitions | Notification read resource plus internal write model using ObjectReference | #841 |
| OAuth callback rejection and token exchange/refresh log attributes | structured telemetry/log events, not wire resources | explicit exception |
| OpenTelemetry spans/metrics and request-log labels | consume already resolved common metadata; no resource envelope | each resource issue, #849 audit |
| webhooks | no product payload exists in the audited revision | inventory again before final merge |

## First-party consumer and generated-artifact inventory

| Consumer/artifact | Required migration | Owner |
| --- | --- | --- |
| `internal/schema/api/schema.json` and schema fixtures | replace flat managed-resource fixtures and compile the new resource/action/projection definitions | #831-#843 |
| API and Admin API Swagger/OpenAPI output under `internal/service/*/swagger` | regenerate against `/api/v1` resource objects; remove obsolete adapters | #833, #849 |
| JavaScript SDK under `sdks/js` | discriminated resource/list/action types, serializers, references, pagination, and JWT Actor helpers | #844 |
| Admin UI under `ui/admin` | queries, stores, forms, tables, search, fixtures, stories, mocks, tests, and screenshots | #845 |
| Marketplace UI under `ui/marketplace` | connector generations, connection setup, actor context, fixtures, stories, mocks, tests, and screenshots | #846 |
| Terraform provider under `terraform/provider` | map idiomatic HCL to resources; remove connector metadata-stripping workaround; preserve secret state rules | #847 |
| CLI under `cmd/cli` | JSON/YAML input/output, actor signing, resource printers, proxy/setup commands, and examples | #838, #848 |
| `dev_config`, Docker Compose, Helm/Kustomize/deployment configuration, demos | replace embedded resource data and document destructive recreation | #848 |
| integration/load tests and golden fixtures | new envelopes only on fresh SQLite/PostgreSQL environments | #848 |
| Starlight docs and README references | update concepts, API/config/JWT/Terraform/deployment examples and destructive migration note | #849 |
| redaction, audit/request logging, caches, telemetry label projection, generic ID/name/namespace helpers | audit for field-path and reference changes | owning backend issues, #849 final audit |

Terraform's HCL does not need literal `api_version`, `kind`, `metadata`, and
`spec` blocks when that would make the provider unidiomatic. The provider owns a
deterministic mapping to the API resource and exposes computed metadata/status
where Terraform users need it.

## Implementation sequence

1. [#831](https://github.com/rmorlok/authproxy/issues/831) adds shared metadata,
   validation/defaulting helpers, lists, references, conditions, and GVK
   decoding.
2. [#832](https://github.com/rmorlok/authproxy/issues/832) separates Connector
   identity from provider definition and makes encrypted storage a clean break.
3. [#833](https://github.com/rmorlok/authproxy/issues/833) changes common v1
   transport, list, error, binding, redaction, and OpenAPI machinery in place.
4. [#834-#843](https://github.com/rmorlok/authproxy/issues?q=label%3Aproject%3Ak8s-resource-api)
   migrate managed resources, JWT actors, actions, projections, internal task
   payloads, and every backend route family above.
5. #844-#848 migrate the JavaScript SDK, UIs, Terraform provider, CLI,
   configuration/deployment fixtures, demos, and integration tests.
6. [#849](https://github.com/rmorlok/authproxy/issues/849) updates public
   documentation and generated artifacts, runs the old-shape audit, validates
   fresh SQLite and PostgreSQL environments, runs the full test matrix and
   `./scripts/preflight.sh`, and opens the final PR to `main`.

## Final audit rules

The feature branch is not complete until all of the following are true:

- Every managed `/api/v1` response uses `authproxy.net/v1alpha1` and its target
  resource or list kind.
- Create and patch requests use the same resource contract under explicit
  operation policies; no flat create/update DTO survives.
- No managed identity, namespace, generation, release state, label, annotation,
  timestamp, or status is duplicated into Connector `spec.definition` or its
  encrypted database representation.
- No old flat Actor claim decoder remains.
- Every registered route is still represented in this inventory, including
  explicit unwrapped exceptions.
- Every serialized config, token, task, workflow, event, and projection named
  here is migrated or deliberately unchanged according to its classification.
- All first-party consumers use metadata/spec/status or their documented
  action/protocol projections.
- Existing environment reset instructions are prominent; there is no connector
  conversion job or legacy database test.
- Generated schemas and Swagger are current, documentation builds, UI PRs have
  screenshots, fresh SQLite/PostgreSQL tests pass, and repository preflight is
  green.
