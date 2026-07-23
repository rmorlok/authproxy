# OAuth scope expansion requires reauthentication

## Fixture

- An OAuth2 connector at version 1 requests the required `read` scope.
- A user authorizes the connection and the provider grants `read`.
- Version 2 adds the required `write` scope.

## Flow

1. Publish version 2 and script its refresh response to grant only `read`.
2. Migrate the connection to version 2 through `_migrate_version` and wait for
   the durable workflow.
3. Start `_reauth`, inspect the generated authorization URL, and complete the
   callback after granting `read write`.

## Assertions

- The migration requests `read write`, persists the actual `read` grant, and
  leaves the configured connection unhealthy with one `auth_required`
  notification.
- Reauthorization requests `read write`, replaces the migration-time token,
  records both the requested and granted `read write` scopes, restores health,
  and resolves the notification.
- The connection remains usable through the proxy after reauthorization.
