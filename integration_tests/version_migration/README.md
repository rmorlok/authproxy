# Connector version migration integration tests

This package contains end-to-end scenarios for single-connection connector
version migration. The tests define connectors, create real connections,
publish newer connector versions, migrate those connections through
`POST /connections/{id}/_migrate_version`, and assert resulting connection
state, auth/config behavior, probes, and notifications.

Scenario files:

- API-key configure migration: tracked by #739.
- API-key preconnect migration: tracked by #739.
- OAuth required-scope reauth migration: tracked by #740.
- OAuth required-scope rollback migration: tracked by #740.
- No-auth defaults and probe migration: tracked by #741.

Shared harness work is tracked by #738.
