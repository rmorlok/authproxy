# OAuth disconnect revocation

This test verifies proxy-initiated OAuth disconnect and revocation. It uses the provider `/test/*` control plane to complete authorization without a browser, then calls the AuthProxy disconnect API and drains the queued `core:disconnect_connection` task with an in-test Asynq worker.

The fixture connector enables OAuth revocation and configures the provider's expected revoke form credentials. It revokes the refresh token, which is sufficient to invalidate provider-side credentials while exercising AuthProxy's local token cleanup path.

The assertions cover:

- `POST /connections/{id}/_disconnect` returns a disconnecting connection and task id;
- the disconnect task calls the provider revocation endpoint with the stored refresh token;
- successful provider revocation tombstones the local OAuth token row and soft-deletes the connection;
- future proxied calls fail with `connection not found`, forcing a reconnect instead of using stale credentials;
- provider revocation failures are retried and then the product policy proceeds with local disconnect so the connection cannot remain stuck in `disconnecting`.
