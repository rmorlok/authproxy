# OAuth multiple connections

Companion specification for `multiple_connections_test.go`. It covers
multiple independent OAuth connections to the same provider.

## Coverage

`TestOAuth2MultipleConnections_SameTenantRefreshIsolation` creates two
connections in the same tenant against the same connector and two provider
accounts. Each authorization-code flow must persist a distinct token row and a
distinct refresh token. Expiring and proxying through connection A must refresh
only A; connection B's active token row and refresh token must remain unchanged.
The test then expires B and proves it refreshes independently.

`TestOAuth2MultipleConnections_DifferentTenantsSameProviderAccount` creates two
tenant namespaces, connects both tenant actors to the same provider account, and
asserts the resulting connections live in their requested namespaces with
distinct token rows. The test disables provider refresh-token rotation for this
case because the provider intentionally returns the same refresh token for the
same client/user pair; the AuthProxy contract under test is that each tenant has
its own connection and active token row. Each tenant connection is force-expired
and proxied as that tenant actor; refreshing one tenant must not mutate the
other tenant's active token row.

## Harness notes

The tests use the go-oauth2-server `/test/authorize` shortcut because this
scenario is about storage and refresh isolation, not browser login or consent.
Refresh behavior is still real: the proxy posts to the provider token endpoint,
receives rotated provider tokens, persists replacement token rows, and forwards
the original resource request.

Provider request inspection is filtered to `grant_type=refresh_token` calls for
the registered client. The expected count is exactly one refresh call per
force-expired connection.
