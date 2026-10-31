# API Schema

This package contains endpoint-specific API request and response DTOs plus 
shared API transports. These types describe wire contracts at AuthProxy route 
boundaries that are not already canonical resource contracts.

Canonical resources and their resource-specific patches live in 
`internal/schema/resources/...`; routes use those types directly when the entire
body is the resource or patch. Do not add aliases here merely to rename a 
resource for a particular endpoint. API models may compose shared primitives 
from `internal/schema/common` and resource models from 
`internal/schema/resources/...`. Resource packages must not import this package.

Auxiliary route DTOs also belong here: session initiation, request-events list
envelopes, task status and monitoring responses, typed imperative actions, and
read-only projections such as connection setup options and OAuth scopes. Keep
route packages focused on binding, validation,
authorization, and conversion to/from service-layer types.

OpenAPI-only generator adapters live under `internal/schema/api/openapi`. Keep
those adapters thin: they may compose API DTOs and canonical resources for
documentation, but no runtime contract or behavior may depend on them.

Route handlers should import endpoint-specific DTOs from this package and
canonical resources from their resource package, or convert those contracts to
runtime/core/database models. Do not add new endpoint-specific `*RequestJson`
or `*ResponseJson` structs under `internal/routes`; preflight rejects that so
contract ownership stays clear.

## Proxy response metadata

The wrapped proxy response carries required `sendEvidence` alongside its
existing status, headers, and body fields. `ProxyResponseJson`, the OpenAPI
projection, and the runtime `iface.ProxyResponse` use the shared
`common.SendEvidence` vocabulary. The runtime body fields retain their existing
serialization, including JSON null for unused `bodyRaw` and `bodyJson`; the
reference schema accepts those values.

The shared `ErrorResponse.sendEvidence` is optional because only wrapped-proxy
errors populate it. Unrelated API errors retain their existing shape. The route
uses trusted request-local evidence, never provider response fields, and keeps
the existing outer HTTP status/error behavior. See the public
[proxying guide](https://docs.authproxy.net/sdks/proxying/#target-send-evidence)
for the cumulative semantics and client retry considerations.

## Tool management lists

`ListToolsResponseJson` is the `ToolList` envelope returned by `GET /tools`.
Its items are complete management `Tool` resources, including executor
definitions, so it requires the management `tools:list` grant. Agents use the
discovery descriptors below instead; an invocation grant never reveals this
list. `CreateToolRequest` and `UpdateToolRequest` in `schema.json` are the
canonical Tool resource and `ToolPatch`, not separate DTOs.

`ListToolSetsResponseJson` is the `ToolSetList` envelope for both
`GET /toolsets`, which returns each ToolSet's default projection (its primary
generation, otherwise its draft), and `GET /toolsets/{id}/generations`, which
returns one item per generation, newest first. Every item projects the current
shared connection selector. `CreateToolSetRequest` and `UpdateToolSetRequest`
are the canonical ToolSet resource and `ToolSetPatch`; generation PATCH and
POST bodies use `ToolSetPatch` too and reject metadata and selector fields.

These lists accept `orderBy=<field> [asc|desc]`, like other resource lists.
`ConnectionToolSetBindingList` is returned by `GET /connections/{id}/toolsets`.
Each item identifies one assignment by its connection and logical ToolSet
references. `status.appliedGeneration` is absent until an inventory is installed,
and the `Installed` condition is `False` with reason `InventoryPending` until then.
The route does not report `Ready` or MCP catalog status yet, so those are omitted.
Bindings page by ToolSet ID with a keyset cursor and have no `orderBy` option.

`GET /tools` and `GET /toolsets` sort by `id`, `namespace`, `name`,
`created_at`, or `updated_at`, defaulting to `id`. Generation lists sort by
`generation`, `state`, `created_at`, or `updated_at`, defaulting to newest
first. `name` and `state` use each database's text collation. Cursors keep the
original filters and ordering, so `orderBy` is ignored on a continuation.

## Tool discovery

`ToolDescriptor` is the usage information an agent can read through a tool grant.
It contains the Tool ID and revision, display name and description, minimal
connection identity, readiness, and optional hints and schemas. It has no fields
for executable definitions, permission verbs, credentials, connection
configuration, or arbitrary metadata. Management resources remain separate.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ToolDescriptor
toolId: tol_01example0000001
revision: 7
name: list-events
description: List calendar events for a team.
connection:
  id: cxn_01example0000001
  name: work-calendar
readiness:
  state: ready
hints:
  readOnly: {value: true, provenance: administrator}
  idempotent: {value: false, provenance: provider}
schema: full
inputSchema:
  type: object
  properties:
    team: {type: string}
  required: [team]
examples: [{team: engineering}]
usage: Use the connection name to select the intended account.
```

Generated tools add `toolSet: {id, name, generation}`. MCP tools can also include
their connection's positive `catalogRevision`; a catalog revision requires a
ToolSet identity. These fields describe the captured definition's provenance.

Readiness is `ready`, `unavailable`, or `unknown`, with an optional extensible
`reason` and safe `message`. Optional `connectionHealth: {state: unhealthy}`
records probe/credential health separately: an unhealthy probe can coexist with
a ready tool. An omitted health observation makes no health claim. Readiness
does not promise that a later invocation will succeed.

Each behavioral hint has its own `administrator` or `provider` provenance, so
overriding one imported hint does not imply that all hints are trusted. Explicit
false differs from an omitted hint. Hints never grant permission or enable
automatic retries.

The required `schema` field identifies the descriptor's detail level:

- `none` omits input and output schemas, examples, and usage guidance.
- `compact` includes an input schema and optional output schema. The producer
  removes verbose descriptions/examples while preserving validation constraints.
- `full` includes unchanged schemas and can add bounded usage guidance and
  `examples`, an array of invocation argument objects.

The API validator checks JSON shapes, not JSON Schema semantics or example
conformance. Producers compile schemas before publication, enforce size bounds,
and construct safe projections. Raw schemas and examples preserve nested nulls
and number precision through JSON/YAML. A boolean output schema is supported.

`ToolSearchQuery` selects one page using `query`, `labelSelector`, optional
`toolSetId`, `schema`, `includeUnavailable`, `limit`, and `continue`.
`WithDefaults()` selects `schema=none` and a limit of five; explicit limits must
be between 1 and 100. `includeUnavailable` defaults to false and can include
permitted tools with unavailable or unknown readiness. Connection selection uses
`labelSelector=apxy/cxn/-/id=cxn_01example0000001`; there is no separate connection
list option. Selectors narrow permissions and are applied before pagination.

`ToolDescriptorList` uses the shared v1alpha1 list envelope and an opaque
`metadata.continue` token. Single describe responses use `ToolDescriptor` itself.
`AgentConnectionList` similarly returns minimal `AgentConnection` summaries with
`id`, `name`, and server-computed `canProxy`. Visibility through a tool grant does
not imply proxy access.

These are contracts for the future discovery service. The service must parse
the existing label-selector syntax, authorize before ranking/counting/pagination,
derive connection labels, bind cursors to caller/query/catalog context, and
evaluate `canProxy` using the actor/JWT intersection. DTO validation performs
none of those runtime decisions. It also does not sanitize provider descriptions;
clients must treat descriptions, schemas, examples, and guidance as data.

## Connection ToolSet bindings

`ConnectionToolSetBinding` reports one ToolSet's installation on a connection.
The planned `GET /connections/{id}/toolsets` endpoint returns these objects in a
`ConnectionToolSetBindingList`, with an opaque `metadata.continue` cursor.
The binding uses the standard resource envelope, but all fields are produced by
the server. It is not a resource callers create or edit.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectionToolSetBinding
metadata:
  namespace: root.product.user-1
spec:
  connectionRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    id: cxn_01example0000001
  toolSetRef:
    apiVersion: authproxy.net/v1alpha1
    kind: ToolSet
    id: tls_01example0000001
status:
  appliedGeneration: 4
  conditions:
    - type: Installed
      status: "True"
      observedGeneration: 4
      lastTransitionTime: 2026-10-11T10:00:00Z
    - type: MigrationSucceeded
      status: "False"
      observedGeneration: 5
      lastTransitionTime: 2026-10-11T10:01:00Z
      reason: ImportFailed
      message: The previous generation remains installed.
```

The references identify the pair by immutable IDs. They have no generation;
`status.appliedGeneration` records the inventory currently installed. A failed
migration leaves that value unchanged and reports the attempted target through
the condition's `observedGeneration`. A rollback can target an older generation.

Before installation, `appliedGeneration` is absent. An `Installed` condition can
report `False` or `Unknown` with a reason such as `WaitingForPrimary` or
`ConnectionNotConfigured`. `Ready` reports execution readiness separately.
Condition types are extensible and unique within the binding. Omitted conditions
make no claim about their state.

`metadata.namespace` is the connection namespace. If a reference includes a
namespace, the connection reference must match it and the ToolSet reference must
be in that namespace or an ancestor. Bindings have no separate public ID, name,
or generation in metadata. The service resolves references and enforces access.

MCP bindings can also include `status.catalog`:

```yaml
catalog:
  lastAttemptAt: 2026-10-11T10:02:00Z
  lastComplete:
    revision: 7
    sourceDigest: sha256:opaque-catalog-digest
    lastSuccessAt: 2026-10-11T10:00:01Z
    freshness: stale
    contextMatches: "False"
  diagnostics:
    - severity: error
      code: catalog.refresh.failed
      message: The last complete catalog was retained.
```

Catalogs belong to individual connections. A failed refresh advances
`lastAttemptAt` and reports diagnostics while retaining `lastComplete` and marking
it stale. A failed first discovery can report an attempt and diagnostics without
a complete catalog or applied generation. A complete catalog requires both an
attempt timestamp and the enclosing binding's applied generation. The attempt
time is its start, so it can precede the successful completion time.

`freshness` and `contextMatches` describe different things. A stale catalog can
remain usable if its authentication context still matches. `False` or `Unknown`
context agreement makes it unavailable even when the last enumeration is fresh.
Retaining the snapshot lets administrators inspect that state; it does not grant
execution permission. Invocation admission must recheck the live context.

Validation checks the projection's shape. Routes, reconciliation, migration and
refresh execution, atomic inventory activation, freshness calculation, and context
checks follow in later slices. Producers must bound and redact diagnostics;
credentials, context fingerprints, internal epochs, and raw catalog contents do
not belong in this response.

## ToolSet migration and catalog refresh actions

These connection management actions select one ToolSet binding and start
background work:

| Planned endpoint | Action kind | ToolSet reference |
| --- | --- | --- |
| `POST /connections/{id}/_migrateToolSetGeneration` | `ConnectionToolSetGenerationMigration` | Selects an exact generation. |
| `POST /connections/{id}/_refreshToolSet` | `ConnectionToolSetRefresh` | Selects the logical ToolSet, without a generation. |

For example, migrate one connection's binding to generation 5:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectionToolSetGenerationMigration
metadata:
  target:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    id: cxn_01example0000001
spec:
  toolSetRef:
    apiVersion: authproxy.net/v1alpha1
    kind: ToolSet
    id: tls_01example0000001
    generation: 5
  timeoutSeconds: 600
```

The target generation must be positive. An older active generation can be a
rollback target; the contract imposes no increasing-generation rule. Both
references follow the existing management convention: an ID or a namespace/name
pair, with a typed `apiVersion` and `kind`. A Connection target never carries a
generation.

For an MCP catalog refresh, use `kind: ConnectionToolSetRefresh` with the same
Connection target and omit the ToolSet generation:

```yaml
spec:
  toolSetRef:
    apiVersion: authproxy.net/v1alpha1
    kind: ToolSet
    id: tls_01example0000001
```

Refresh uses that binding's current source configuration. It does not migrate
the binding or refresh an OpenAPI draft. Both actions accept an optional positive
`timeoutSeconds` for the background operation. Omission uses the service default;
the value does not tell the HTTP request to wait for completion.

Responses keep the same action kind, target, and safe request spec, and add:

```yaml
status:
  taskId: opaque-encrypted-tracking-token
```

`taskId` follows connector migration: it is an actor-bound tracking token for
`GET /tasks/{token}`, not a raw workflow or queue identifier. It acknowledges
accepted durable work, not successful inventory activation. Poll task status for
completion and inspect the binding for its installed generation, migration
conditions, or retained MCP catalog. Requests must omit server-owned status.

Use strict JSON/YAML decoding and the action's `ValidateRequest` or
`ValidateResponse` method with its kind constant. Validation checks references,
generation/timeout shape, and the response token; it does not resolve references
or authenticate tokens. Routes and workers must enforce URL-target agreement,
management permissions, binding/source eligibility, published-generation state,
and atomic activation with context/epoch checks. These runtime behaviors and
OpenAPI draft refresh remain later implementation work.

## Tool invocation

`ToolInvokeRequest` targets a Tool by its immutable ID. Its `spec.arguments`
must be a JSON object; use `{}` when there are no inputs. Values stay intact,
including nested nulls and large numbers. The contract validator checks this
shape; the future admission path will validate against the tool's input schema.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ToolInvoke
metadata:
  target:
    apiVersion: authproxy.net/v1alpha1
    kind: Tool
    id: tol_01example0000001
spec:
  expectedRevision: 7
  arguments:
    team: engineering
```

`expectedRevision` is optional and must be positive when supplied. It means
“execute only if the current revision is still 7.” A mismatch will return HTTP
409 `TOOL_REVISION_CHANGED` before execution. Omit it to use the current revision.
It does not select an old revision or prevent repeated execution on retry.
The target accepts an ID only: names, namespaces, and generations are rejected.

`ToolInvokeResponse` uses the same kind and target, an empty `spec`, and the rich
result in `status.result`. Keeping the response spec empty avoids echoing request
arguments. The result's `toolId` must agree with the target ID.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ToolInvoke
metadata:
  target:
    apiVersion: authproxy.net/v1alpha1
    kind: Tool
    id: tol_01example0000001
spec: {}
status:
  result:
    toolId: tol_01example0000001
    revision: 7
    invocationId: tiv_01example0000001
    isError: false
    structuredContent: {team: engineering}
    content: []
    metadata: {connectionId: cxn_01example0000001}
```

Use strict JSON/YAML decoding followed by `ValidateRequest(ToolInvokeActionKind)`
or `ValidateResponse(ToolInvokeActionKind)`. Request status is server-owned;
response status and its result are required. These types follow existing action
decoding: explicit null for `expectedRevision` or request `status` is treated as
omission. Constructors wrap supplied values without copying or validating them.

This slice defines contracts only. Routes, permission checks, URL-target
agreement, execution snapshots, and revision comparison follow in the invocation
implementation. Producers remain responsible for keeping connection credentials
and resolved configuration out of result content and diagnostics.

## Tool results

`ToolResult` defines the outcome that a `ToolInvoke` action returns in
`status.result`. It is an endpoint contract, not a stored resource or background
task. The surrounding `v1alpha1` API versions the result and its content union;
there is no separate per-block version field. This package defines validation,
serialization, and cloning; invocation routes and execution follow later.

```json
{
  "toolId": "tol_01example0000001",
  "revision": 7,
  "invocationId": "tiv_01example0000001",
  "isError": false,
  "structuredContent": null,
  "content": [
    {"type": "text", "text": "The provider returned JSON null."},
    {"type": "resource_link", "uri": "provider://reports/42", "name": "Report"}
  ],
  "metadata": {"connectionId": "cxn_01example0000001"}
}
```

`structuredContent` accepts any JSON value. Omitting it differs from supplying
`null`; JSON/YAML round trips preserve both that distinction and large numbers.
`content` is always an array, including `[]`, and preserves block order. The
connection ID is required. Imported results can include positive
`toolSetGeneration` and `catalogRevision` values in `metadata`; a catalog
revision requires its ToolSet generation. `tiv_` identifies one invocation for
correlation and does not imply an asynchronous task or retrieval endpoint.

| Content type | Payload |
| --- | --- |
| `text` | Required `text`, which can be empty. |
| `image`, `audio` | Required base64 `data` and `mimeType`. |
| `resource_link` | Required `uri` and `name`; optional title, description, media type, size hint, and icons. |
| `resource` | Required `resource` object with `uri`, optional `mimeType`, and exactly one of `text` or base64 `blob`. |

The normalized union rejects mixed variants. Empty text, binary payloads, and
link names are preserved. Resource and icon URIs can use custom schemes. They
identify content without authorizing a fetch; an invocation-scoped URI for
inline bytes does not promise a download endpoint. Provider size hints do not
determine execution byte accounting.

All blocks can carry `annotations`: `audience` (`user` and/or `assistant`),
`priority` from zero to one, and an opaque `lastModified` string. These are
presentation hints. Optional `provider` objects preserve redacted extension
metadata separately at result, block, and embedded-resource levels. Protocol
adapters map upstream metadata into these fields; it does not become AuthProxy
instructions or authorization. The field shapes preserve the rich blocks in
the [MCP schema](https://modelcontextprotocol.io/specification/2026-07-28/schema),
without making MCP SDK types part of this API.

Failures set `isError: true` and require an `error` object. Success forbids it.
The error includes nonblank `code` and `message`, optional JSON `details`, a
required `retryable` boolean, and `executionState`:

- `notStarted`: execution has not started.
- `mayHaveExecuted`: upstream effects are possible or uncertain.
- `completed`: execution completed, even if result processing later failed.

Error results can retain the original rich blocks and structured content.
`retryable` is advice, not a guarantee that replay is safe. Error details also
preserve omission versus explicit JSON null. Producers own code selection,
reserved-code policy, redaction, output-schema checks, and encoded/decoded byte
limits. Validation checks the public shape and encodings; it does not sanitize
messages or run output schemas. Existing HTTP errors before result formation
remain separate from this outcome contract.

## Notifications
Notifications and search make their non-resource semantics explicit here.
`NotificationJson` is a read-only, resource-shaped projection of a durable
notification row because its `status.viewed` and `status.action` values are
calculated for the authenticated actor; it is not a client-managed desired
resource. Search items are heterogeneous summaries, not partial resources.
`SearchResultList` therefore carries typed `resourceRef` values and puts
truncation/incompleteness observations in projection metadata. Notification
view mutations use typed action contracts.

## Request Events
Request events are immutable, resource-shaped observations rather than
client-managed desired resources. `RequestEventJson` uses resource metadata for
the event identity, namespace, label snapshot, and creation time. Its `spec`
contains the observed exchange, typed references to attributed resources, and
optional captured protocol data. Capture fields remain explicitly tagged for
the API secret-replay policy. `RequestEventList` uses list metadata for its
opaque continuation token and exact result total.

## Tasks/Workflows and Related Operations
Tasks, queues, workflow instances, and their related operational records are
read-only projections over Asynq and go-workflows, not AuthProxy-managed desired
resources. They still use typed v1alpha1 objects so their identity, spec, and
observed status are unambiguous. Queue pause/run/delete operations and workflow
cancel/delete operations return typed action results. In particular,
`WorkflowInstance.metadata.id` is the unique execution ID and
`WorkflowInstance.spec.instanceId` is the logical workflow instance ID; both
are required by the backend protocol.

The route contract inventory for these projections is:

| Endpoint | Response contract |
|---|---|
| `GET /tasks/{encryptedTaskInfo}` | `Task` |
| `GET /task-monitoring/queues` | `TaskQueueList` |
| `GET /task-monitoring/queues/{queue}` | `TaskQueue` |
| `GET /task-monitoring/queues/{queue}/history` | `TaskQueueHistory` |
| `GET /task-monitoring/queues/{queue}/tasks/{state}` | `TaskExecutionList` |
| `GET /task-monitoring/queues/{queue}/tasks/{state}/{taskId}` | `TaskExecution` |
| `GET /task-monitoring/servers` | `TaskServerList` |
| `GET /task-monitoring/scheduler-entries` | `TaskScheduleList` |
| `POST /task-monitoring/queues/{queue}/tasks/{taskId}/_run` | `TaskExecutionRun` action |
| `POST /task-monitoring/queues/{queue}/tasks/{taskId}/_archive` | `TaskExecutionArchive` action |
| `POST /task-monitoring/queues/{queue}/tasks/{taskId}/_cancel` | `TaskExecutionCancel` action |
| `DELETE /task-monitoring/queues/{queue}/tasks/{taskId}` | `TaskExecutionDelete` action |
| `POST /task-monitoring/queues/{queue}/_pause` | `TaskQueuePause` action |
| `POST /task-monitoring/queues/{queue}/_unpause` | `TaskQueueUnpause` action |
| `POST /task-monitoring/queues/{queue}/archived/_runAll` | `TaskQueueRunAll` action with `spec.state: archived` |
| `POST /task-monitoring/queues/{queue}/retry/_runAll` | `TaskQueueRunAll` action with `spec.state: retry` |
| `DELETE /task-monitoring/queues/{queue}/archived` | `TaskQueueDeleteAll` action with `spec.state: archived` |
| `DELETE /task-monitoring/queues/{queue}/completed` | `TaskQueueDeleteAll` action with `spec.state: completed` |
| `GET /workflow-monitoring/instances` | `WorkflowInstanceList` |
| `GET /workflow-monitoring/instances/{instanceId}/{executionId}` | `WorkflowInstance`; history is expanded in `status` |
| `GET /workflow-monitoring/instances/{instanceId}/{executionId}/tree` | `WorkflowInstance`; children are expanded in `status` |
| `GET /workflow-monitoring/instances/{instanceId}/{executionId}/history` | `WorkflowHistoryEventList` |
| `POST /workflow-monitoring/instances/{instanceId}/{executionId}/_cancel` | `WorkflowInstanceCancel` action |
| `DELETE /workflow-monitoring/instances/{instanceId}/{executionId}` | `WorkflowInstanceDelete` action |

Three private protocols are intentionally not resource envelopes. Asynq task
payload bytes are decoded only by their registered task handlers. Persisted
go-workflows arguments and history are owned by go-workflows and use durable
`.v1` workflow registration names. The encrypted `internal/tasks.TaskInfo`
value is an actor-bound locator token containing backend IDs. The current
payloads contain scalar IDs and parameters rather than embedded API resource
objects, so this migration does not add a second payload version or conversion
job. If a future private payload embeds a resource, its task/workflow name and
payload type must be versioned before deployment.
