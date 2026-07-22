# API-key preconnect migration

## Fixture

- Create a draft bearer API-key connector through the connector API.
- Promote version 1 to primary.
- Create a connection through `_initiate`, submit the API key, and run verify against the stub upstream so the connection is configured and healthy.
- Publish version 2 with one new required preconnect field, `region`.

## Flow

1. Start the core workflow worker in-process.
2. Call `POST /connections/{id}/_migrate_version` targeting version 2.
3. Wait for the durable task to complete.
4. Rotate the stub upstream to require a new API key.
5. Start reauth through `POST /connections/{id}/_reauth`.
6. Submit the `select-region` preconnect form, then submit the API-key credential form.
7. Run verify against the stub upstream.

## Assertions

- Migration switches the connection to connector version 2.
- The connection remains configured but becomes unhealthy because required auth-time data is missing.
- Exactly one active high-level `auth_required` notification is visible for the actor.
- The notification action points to reauth and does not use host-internal naming.
- Reauth stores `region`, replaces the active API key, returns the connection to healthy, clears setup state, and resolves the notification.
