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

Labels, annotations, and namespace encryption-key assignment are not auxiliary
subresources. They are fields on the parent resource's `metadata` or `spec` and
must use that resource's patch contract. Do not reintroduce generic key/value
route DTOs or per-key mutation endpoints.

OpenAPI-only generator adapters live under `internal/schema/api/openapi`. Keep
those adapters thin: they may compose API DTOs and canonical resources for
documentation, but no runtime contract or behavior may depend on them.

Route handlers should import endpoint-specific DTOs from this package and
canonical resources from their resource package, or convert those contracts to
runtime/core/database models. Do not add new endpoint-specific `*RequestJson`
or `*ResponseJson` structs under `internal/routes`; preflight rejects that so
contract ownership stays clear.

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
