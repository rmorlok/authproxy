# Apply infrastructure

The apply implementation is delivered in stages. `Load` remains an offline
manifest loader (apart from explicit manifest URL downloads). The CLI currently
exposes client dry-run only. `Client` supplies the ordinary authenticated REST
operations for the future reconciler/executor; it does not enable cluster apply
by itself.

`cmd/cli/config.Resolver.ResolveApplyClient` reuses CLI configuration and JWT
signing. It selects the API service normally and the admin API in admin mode,
without silently falling back between them. Callers pass
`DefaultRequestTimeout` (30 seconds) unless overridden; zero disables the
per-request timeout. Client dry-run must not instantiate this client.

`ResolveBatch` performs reads only. It rejects unsupported/admin-only kinds
before requests, resolves IDs or exact namespaced names, follows pagination,
checks supplied identity fields, detects aliases resolving to the same live ID,
and validates create or patch contracts. Explicit IDs and connector generations
must already exist. Namespace names derive a canonical path for direct lookup;
a missing name-addressed Namespace can be created. Connections can only update
existing mutable metadata.

`Create` and `Update` submit one validated operation. They use canonical resource
and patch types, including immutable-field checks and omitted/null/empty patch
semantics. `Update` accepts a calculated canonical patch; callers must perform
three-way reconciliation before using it. Validation of the original document
in `ResolveBatch` is not a generated execution plan. Resource recognition, patch factories, metadata access, ID validation and
canonical lifecycle/merge capabilities come from `schema/registry`. Apply
retains transport, target resolution, and operation policy.

`LiveResource` retains typed responses, the redaction header, and known
write-only field paths. Never derive mutations from masked values or assume
omitted write-only fields are absent on the server. Resources and documents may
contain secrets and must not be logged. API errors expose HTTP status without
reflecting response bodies, and the HTTP client refuses redirects so signing
credentials are not forwarded elsewhere.

Batch dependency ordering, reconciliation/history, automatic retries, and
conditional writes are not implemented in this layer. A successful operation
followed by a lost/invalid response has an uncertain outcome; the client does
not retry mutations. Server validation and authorization remain authoritative,
and ordinary read/patch sequences do not prevent concurrent writes.
