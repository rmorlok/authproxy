# OAuth scope expansion rollback restores the connection

## Fixture

- An OAuth2 connector at version 1 requests the required `read` scope.
- Version 2 adds the required `write` scope.
- The migration-time refresh is scripted to grant only `read`, so version 2
  requires reauthentication.

## Flow

1. Migrate from version 1 to version 2 and confirm the connection requires
   reauthentication.
2. Target version 1 with `_migrate_version` and wait for the rollback
   workflow.

## Assertions

- Rollback restores connector version 1, the configured state, and healthy
  health state without requiring a user callback.
- The target-version refresh records requested and granted `read` scope.
- The `auth_required` notification resolves and the proxy request succeeds.
