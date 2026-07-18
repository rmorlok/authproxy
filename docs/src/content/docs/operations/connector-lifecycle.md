---
title: Connector Lifecycle Operations
---

AuthProxy exposes connector-wide lifecycle operations for administrative cleanup:

- `POST /connectors/{id}/_disconnect_all`
- `POST /connectors/{id}/_archive`

Both endpoints start go-workflows-backed background work and return a `task_id` that can be polled with `GET /tasks/{task_id}`. They run on the normal worker service, share the application database through the workflow backend, and use the same task polling contract as other long-running work.

## Choosing An Operation

Use **disconnect all** when a connector should remain available, but every current connection for that connector version should be disconnected and removed. This is the operational cleanup action for compromised credentials, connector configuration problems, or a deliberate reset before users reconnect. It does not change connector version state.

Use **archive** when a connector version is being retired. Archive first prepares the connector versions so no new connections can be created, then disconnects existing connections, then archives all versions of the connector after the disconnect work reaches a terminal state.

The archive preparation step moves draft versions to `archived` and moves the current primary version to `active`. Moving the primary version out of `primary` prevents new connections while existing connections are being cleaned up. After cleanup, the finalize step moves every remaining version for that connector to `archived`.

## Request Shape

Both endpoints accept an optional JSON body:

```json
{
  "timeout_seconds": 600
}
```

`timeout_seconds` must be greater than zero. If omitted, AuthProxy uses the default connector lifecycle timeout of 600 seconds.

The caller needs `connectors:disconnect_all` permission for disconnect-all and `connectors:archive` permission for archive, scoped to the connector namespace and id.

## Task Polling

The start response contains a secure task token:

```json
{
  "task_id": "encrypted-task-info",
  "connector_id": "cxr_..."
}
```

Poll it with:

```http
GET /tasks/{task_id}
```

Workflow-backed connector lifecycle tasks usually report these states:

- `active`: the workflow is still running.
- `completed`: the workflow reached its intended terminal state.
- `failed`: the workflow was canceled, terminated, or completed with an error.
- `unknown`: the workflow state could not be resolved.

For compatibility with Asynq-backed tasks, the task schema also includes `pending`, `scheduled`, and `retry`. Treat those as in-progress states when building generic task polling clients.

## Timeout And Forced Terminal Semantics

Disconnect-all starts one child `core.connection.disconnect.v1` workflow for each relevant connection. Relevant connections are in `setup`, `configured`, `disabled`, or `disconnecting` state.

The parent workflow keeps a five-second reserve from the requested timeout when scheduling child connection disconnect workflows. For example, a 600-second connector lifecycle timeout gives each child disconnect workflow up to 595 seconds and leaves the parent time to force any remaining connections to a terminal local state.

When the parent timeout expires, or when a child disconnect workflow returns an error, the parent forces those remaining connections to `disconnected` and soft-deletes them. This means a `completed` connector lifecycle task means AuthProxy local state is terminal. It does not guarantee that every third-party token revocation succeeded; provider revocation failures can be exhausted by the child workflow and then finalized locally so the connector-level operation can finish.

Archive uses the same disconnect-all workflow as a child. Its requested timeout is passed through to that child operation, so the timeout bounds connection cleanup. The archive preparation and finalize steps are short database state transitions that run immediately before and after the child disconnect-all workflow.

## Concurrency

Connector lifecycle workflow instance ids are deterministic:

- `core.connector.disconnect_all.v1:<connector_id>`
- `core.connector.archive.v1:<connector_id>`

This prevents duplicate disconnect-all or archive workflows from running for the same connector at the same time. After a workflow completes, AuthProxy can start a new execution with the same workflow instance id, which allows a connector to be disconnected again after new connections are created.

Archive invokes disconnect-all with the same connector-specific disconnect-all instance id. If a disconnect-all workflow for the connector is already active, starting archive for that connector will fail instead of creating a competing cleanup workflow.

## Durable Workflow Names

The connector lifecycle workflows and activities use durable names that are stored in workflow history:

```text
core.connector.disconnect_all.v1
core.connector.disconnect_all.list_connections.v1
core.connector.disconnect_all.force_remaining.v1
core.connector.archive.v1
core.connector.archive.prepare_versions.v1
core.connector.archive.finalize_versions.v1
```

These names follow the versioning rules in [Workflow versioning](/development/workflows/). Breaking changes must add a new `vN` workflow or activity name and keep the old registration available for at least one deployment window so in-flight customer workflows can continue replaying after an upgrade.

## Verification Matrix

The connector lifecycle integration tests cover the workflow-backed polling path and the final data effects:

- `TestDisconnectRevocation_RevokesProviderTokensAndBlocksFutureProxy`
- `TestDisconnectRevocation_RevocationFailureStillCompletesDisconnect`
- `TestConnectorDisconnectAll_DisconnectsTargetConnectionsOnly`
- `TestConnectorDisconnectAll_RevocationFailureStillCompletes`
- `TestConnectorArchive_ArchivesVersionsAndDisconnectsConnections`
- `TestConnectorArchive_RevocationFailureStillArchives`

Run them against the default PostgreSQL provider:

```bash
cd integration_tests
go test -tags integration ./oauth2 -run 'TestDisconnectRevocation|TestConnectorDisconnectAll|TestConnectorArchive' -count=1
```

Run the same workflow shapes against SQLite:

```bash
cd integration_tests
AUTH_PROXY_TEST_DATABASE_PROVIDER=sqlite go test -tags integration ./oauth2 -run 'TestDisconnectRevocation|TestConnectorDisconnectAll|TestConnectorArchive' -count=1
```
