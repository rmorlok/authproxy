# No-auth defaults and target probes

## Fixture

Create a no-auth connector version 1 with an `existing` HTTP probe and
initiate a connection through the public connection API. The connection has no
credentials or setup steps, so it completes in the configured and healthy
state.

Publish connector version 2 with the same `existing` probe, an additional
`added` HTTP probe, and a configure field named `region` that is required but
has the JSON Schema default `us-east-1`.

## Flow

1. Migrate the configured connection from version 1 to version 2 through
   `POST /connections/{id}/_migrate_version` and wait for the durable workflow
   task to complete.
2. Record requests to the retained and newly added probe endpoints.

## Assertions

- The connection is on version 2, configured, healthy, and has no pending
  setup step or setup error.
- The migration materializes `region: us-east-1` in the encrypted connection
  configuration without requiring user setup.
- Both the retained `existing` probe and the newly introduced `added` probe
  run exactly once. This proves migration runs the target probe set rather
  than only the delta.
- The connection has no active notification.
