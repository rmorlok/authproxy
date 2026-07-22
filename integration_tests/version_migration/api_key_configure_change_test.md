# API-key configure migration

## Fixture

- Create a draft bearer API-key connector through the connector API.
- Promote version 1 to primary.
- Create a connection through `_initiate`, submit the API key, and run verify against the stub upstream so the connection is configured and healthy.
- Publish version 2 with one new required configure field, `workspace`.

## Flow

1. Start the core workflow worker in-process.
2. Call `POST /connections/{id}/_migrate_version` targeting version 2.
3. Wait for the durable task to complete.
4. Submit the `configure-workspace` form with a workspace value.

## Assertions

- Migration switches the connection to connector version 2.
- The connection remains configured and healthy, but records `setup_step=configure-workspace`.
- Exactly one active high-level `setup_required` notification is visible for the actor.
- The notification action points to connection configuration and does not use host-internal naming.
- Submitting the configure step stores `workspace`, clears `setup_step`, leaves the connection healthy, and resolves the notification.
