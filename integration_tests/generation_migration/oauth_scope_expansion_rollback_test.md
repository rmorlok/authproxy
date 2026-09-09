# OAuth scope expansion rollback restores the connection

## Fixture

- An OAuth2 connector at generation 1 requests the required `read` scope.
- Generation 2 adds the required `write` scope.
- The migration-time refresh is scripted to grant only `read`, so generation 2
  requires reauthentication.

## Flow

1. Migrate from generation 1 to generation 2 and confirm the connection requires
   reauthentication.
2. Target generation 1 with `_migrateGeneration` and wait for the rollback
   workflow.

## Assertions

- Rollback restores connector generation 1, the configured state, and healthy
  health state without requiring a user callback.
- The target-generation refresh records requested and granted `read` scope.
- The `auth_required` notification resolves and the proxy request succeeds.
