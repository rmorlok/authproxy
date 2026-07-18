---
title: Workflow Versioning
---

AuthProxy uses [go-workflows](https://cschleiden.github.io/go-workflows/) for durable background work that must survive process restarts, worker rollouts, and customer upgrades. Workflow code is replayed from persisted history, so compatibility is stricter than ordinary request handlers or Asynq tasks.

The go-workflows documentation calls out the two rules that shape AuthProxy's conventions:

- Workflow functions must be deterministic so they can be interrupted, replayed, and resumed. Activities may have side effects and do not need to be deterministic.
- Built-in workflow versioning is intentionally not supported yet. The upstream guidance is side-by-side deployments, with queues available for explicit routing when needed.

AuthProxy therefore versions workflows and activities by durable registration name and keeps old implementations available long enough for in-flight instances to finish after an upgrade.

## Naming

Workflow and activity names are durable API names. They are stored in workflow history and task-tracking tokens, so treat them like database schema identifiers.

Use dot-separated names with a major version suffix:

```text
<area>.<resource-or-process>.<action>.v<major>
```

Current examples:

```text
core.connection.disconnect.v1
core.connection.disconnect.revoke_credentials.v1
core.connection.disconnect.finalize.v1
core.connector.disconnect_all.v1
core.connector.archive.v1
```

Rules:

- Keep names stable once released.
- Use lowercase words separated by dots.
- Put the major version at the end, not in a queue name or function name only.
- Version workflows and activities independently. A workflow may stay at `v1` while calling a new `v2` activity only if replay of already-started workflow histories cannot observe that change.
- Keep Go function names descriptive, but never rely on inferred function names for durable production workflows. Register with `registry.WithName(...)`.

## When To Create A New Version

Create a new workflow name when a change can alter the sequence of commands emitted during replay for an existing in-flight workflow.

Unsafe workflow changes include:

- Adding, removing, or reordering `workflow.ExecuteActivity`, timer, signal, side-effect, or sub-workflow calls.
- Changing an activity name or workflow name used by an existing history.
- Changing branch conditions that decide whether a workflow command is emitted, unless the decision is based only on data already recorded in workflow history.
- Reading time, random values, map iteration order, environment variables, configuration, database state, or network state directly inside workflow code.
- Changing serialized workflow inputs or outputs in a way old histories cannot decode.
- Changing retry semantics in a way that changes whether the workflow emits the next command during replay.

Create a new activity name when a side-effect contract changes in a way old workflows may not tolerate.

Unsafe activity changes include:

- Changing the meaning of existing inputs.
- Removing fields that old callers still send.
- Returning errors for inputs that the old activity accepted.
- Changing idempotency behavior, external API targets, or cleanup semantics.
- Changing return payload shape in a way old workflow code cannot decode.

Safe changes usually include:

- Internal refactors that do not affect workflow command order.
- Logging and metrics changes that use replay-aware workflow helpers where applicable.
- Adding optional fields to activity input structs while preserving old defaults.
- Bug fixes inside an activity when the activity's input, output, idempotency, and external side-effect contract are unchanged.
- Adding a brand-new workflow or activity name without changing existing registrations.

When in doubt, create a new version. The cost of one extra registered implementation is much lower than a non-recoverable replay error on a customer's system.

## Determinism And Side Effects

Workflow code describes orchestration. Activity code performs side effects.

Do this in workflows:

- Call registered activities, timers, signals, side effects, and sub-workflows through go-workflows APIs.
- Use only workflow inputs, prior activity results, signal payloads, and values recorded by workflow history to choose the next command.
- Keep branching simple and explicit.
- Keep workflow inputs serializable and backward compatible.

Do not do this in workflows:

- Call the database.
- Call third-party APIs.
- Use `time.Now`, random generators, UUID generators, goroutines, native channels, or native `select`.
- Iterate over a map unless keys are sorted first.
- Read process configuration or environment variables to decide the command sequence.

Put those operations in activities instead. Activities receive `context.Context`, may perform I/O, and should be idempotent because go-workflows retries activities by default.

## Upgrade Pattern

For a breaking workflow change:

1. Add a new workflow name, for example `core.connection.disconnect.v2`.
2. Add new activity names for any changed side-effect contracts.
3. Register both old and new workflow/activity names.
4. Change starters to create only the new workflow name.
5. Deploy with both versions registered.
6. Wait at least one full customer deployment window, and preferably until monitoring shows no in-flight instances for the old workflow name.
7. Remove old registration in a later deployment.

For a breaking activity change used by an existing workflow:

1. Add a new activity name, for example `core.connection.disconnect.finalize.v2`.
2. If old in-flight workflow histories could reach the activity call, keep the old activity registered.
3. Prefer a new workflow version that calls the new activity.
4. Remove the old activity only after no registered workflow version can schedule it and no in-flight history can replay to it.

## Queues

Queues are not AuthProxy's default versioning strategy. The default strategy is versioned workflow and activity names with old versions registered for a bounded retirement period.

Use separate queues only for exceptional cases, such as:

- A migration where old and new workers must run with different binary versions at the same time.
- Operational isolation for a workflow class with very different concurrency or resource needs.
- A temporary rollout where a subset of workflow instances must be routed to specialized workers.

If queues are used for versioning, the queue name must be treated as an operational routing control, not the durable version identifier. The workflow and activity names still need explicit `vN` suffixes.

## Registration Checklist

Before merging a workflow change:

- Existing workflow and activity names remain registered unless their retirement window has passed.
- New breaking behavior has a new `vN` name.
- Starters use the newest intended workflow name.
- Tests prove the expected workflow/activity names are registered.
- Tests cover at least one replay-sensitive path for the workflow.
- Admin and task-monitoring surfaces can still resolve in-flight instances created before the change.
- Documentation identifies when the old version can be retired.

## Local Checks

Run workflow guardrails locally before merging workflow changes:

```bash
./scripts/check-workflows.sh
```

This script runs focused workflow tests that exercise the disconnect pilot workflow with the go-workflows tester and verify its durable workflow/activity names remain registered. It is also called by `./scripts/preflight.sh`.

The current pinned go-workflows release does not package the analyzer mentioned by newer go-workflows documentation, so AuthProxy relies on these local tests for now. When the dependency exposes an analyzer package that matches the pinned release, add it to this check or CI without replacing the registration tests; the tests protect AuthProxy's versioning convention directly.

## Retirement

Old versions can be removed only when all of the following are true:

- The version has not been started by the current release.
- It has remained registered for at least one full deployment after replacement.
- Workflow monitoring shows no active instances for the old workflow name.
- Any retention/debugging requirement for completed histories has been satisfied.
- No supported customer upgrade path can jump from a release that starts the old version directly to a release that omits the old registration.

When removing an old version, include the workflow name, activity names, replacement version, and evidence used to determine that in-flight instances are gone.

## References

- [go-workflows quickstart](https://cschleiden.github.io/go-workflows/#quickstart) documents deterministic workflows, side-effectful activities, and activity retry behavior.
- [go-workflows workflow versioning FAQ](https://cschleiden.github.io/go-workflows/#workflow-versioning) documents the lack of built-in versioning and recommends side-by-side deployments, with queues as an additional routing tool.
- [go-workflows queues guide](https://cschleiden.github.io/go-workflows/#queues) documents worker queues and queue inheritance behavior.
