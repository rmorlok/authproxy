# Apply infrastructure

The apply implementation is delivered in stages. `Load` remains an offline
manifest loader (apart from explicit manifest URL downloads). The CLI defaults
to cluster execution and supports offline validation with `--dry-run=client`.
`Client` supplies authenticated REST operations and prepares execution batches.

`cmd/cli/config.Resolver.ResolveApplyClient` reuses CLI configuration and JWT
signing. It selects the API service normally and the admin API in admin mode,
without silently falling back between them. Callers pass
`DefaultRequestTimeout` (30 seconds) unless overridden; zero disables the
per-request timeout. Key resources are supported on both services, subject to
server authorization. Client dry-run must not instantiate this client.

`ResolveBatch` performs reads only. It rejects unsupported kinds
before requests, resolves IDs or exact namespaced names, follows pagination,
checks supplied identity fields, detects aliases resolving to the same live ID,
and validates calculated create or reconciliation patch contracts. Explicit IDs 
and connector generations must already exist. Namespace names derive a canonical
path for direct lookup; a missing name-addressed Namespace can be created. 
Connections can only update existing mutable metadata.

`Create` and `Update` submit one validated operation. They use canonical 
resource and patch types, including immutable-field checks and 
omitted/null/empty patch semantics. `Update` accepts a calculated canonical 
patch; callers must perform three-way reconciliation before using it. Validation
in `ResolveBatch` does not retain an execution plan; callers use `Reconcile` on 
each resolved target with their overwrite policy. Resource recognition, patch 
factories, metadata access, ID validation and canonical lifecycle/merge 
capabilities come from`schema/registry`. Apply retains transport, target 
resolution, and operation policy.

`LiveResource` retains typed responses, the redaction header, and known
write-only field paths. Never derive mutations from masked values or assume
omitted write-only fields are absent on the server. Resources and documents may
contain secrets and must not be logged. API errors expose HTTP status without
reflecting response bodies, and the HTTP client refuses redirects so signing
credentials are not forwarded elsewhere.

Automatic mutation retries and conditional writes are not implemented in this
layer. A successful operation
followed by a lost/invalid response has an uncertain outcome; the client does
not retry mutations. Server validation and authorization remain authoritative,
and ordinary read/patch sequences do not prevent concurrent writes.

## Reconciliation and history

`Reconcile(target, ReconcileOptions{Overwrite: true})` returns a validated
`Plan` with `create`, `update`, or `unchanged` operation and warnings. It does
not contact the cluster or mutate inputs. For create, submit `plan.Document`
to `Client.Create`; for update, submit `plan.Target` and `plan.Patch` to
`Client.Update`. Unchanged plans need no write. Batch execution uses these
plans; client dry-run does not resolve live state or run reconciliation.

`authproxy.net/last-applied-configuration` is reserved for apply. Its JSON 
format is `{"version":1,"desired":{...},"secrets":["/spec/keyData"]}`. `desired`
retains input presence after normalization, excluding this annotation and secret
subtrees. `secrets` contains only JSON-pointer presence markers, never values,
masked strings, lengths, or hashes. Entire write-only Actor signing keys and
Key provider configurations are excluded, including file/environment/provider
references. Shared discovery uses canonical `apiwriteonly` and `apiredact`
tags for both history exclusions and live-resource comparison metadata; no
resource-specific path list is maintained in apply. Arrays
containing secrets are excluded in full because elements have no stable 
identity. Secret presence is retained across omissions; it never authorizes 
deletion.

Missing history produces an adoption warning and preserves unspecified fields.
Invalid, unsupported, duplicate-key, unsafe or mismatched history fails closed.
History is included in the same create/PATCH as the resource, with the complete
annotation map checked against the 256 KiB total annotation limit. Planning and
failed writes do not advance history. A lost response after a successful write
still has an uncertain outcome; there is no second history-only request.

Three-way comparison uses previous desired, live and new desired values. Maps
merge by key; omitted formerly managed fields are removed while unmanaged
fields survive. Explicit empty maps relinquish their managed keys. Arrays are
atomic replacements. Labels and annotations are sent as full merged maps to
match REST replacement semantics. Spec object replacements similarly include
preserved live fields. Top-level omissions produce canonical clears where legal
(for example Namespace encryptionKeyRef, RateLimit scope and Actor permissions);
unsupported removals fail patch validation. Explicit null remains distinct from
omission and must be supported by the resource contract.

With `Overwrite: false`, drift in a managed field conflicts if the planned
change would overwrite it; messages identify paths without showing values.
With `true` (the CLI default), desired values restore managed fields. Explicit
secrets are always submitted, including supported explicit clears, because
write-only or masked values cannot be compared safely. Omitted secrets are
preserved. A replacement requiring a masked secret fails unless the manifest
supplies that credential. Known equal connector definitions are omitted from
patches, avoiding unnecessary generations; unspecified release intent is 
preserved. Effective typed comparison handles API serialization that omits empty
values.

`Load` accepts `ValidationStrict` (default), `ValidationWarn`, or
`ValidationIgnore`. Warn/ignore remove unknown fields using the registry's
embedded resource schema before strict typed decoding. Warn reports counts and
source locations through `Options.Warn`. Arbitrary map entries remain intact.
Malformed input, duplicate keys, invalid identity, server-owned fields, wrong
types and redacted placeholders always fail. The CLI exposes these as
`--validate=strict|warn|ignore`; `--overwrite` is accepted but has no effect on
client dry-run.

## Batch preparation and execution

`Client.Prepare(ctx, documents, options)` resolves every target and validates
all reconciliation plans before writes. It discovers explicit typed
`meta.ObjectReference` values through `registry.References`, verifies external
references and namespace prerequisites, and produces a stable topological order.
Use the earliest ready input when multiple resources can run. Selected input
must include prerequisites being created, or those prerequisites must already
exist in the cluster; unselected manifests are not silently added to the batch.

Namespace membership and parent edges depend on namespace **creation**. An
existing namespace need not finish an update before resources can be placed in
it. This allows a new key in an existing namespace to precede an encryption-key
reference update. A new namespace that references a key being created inside
it forms a cycle and fails before any write. Explicit reference edges also order
updates to referenced resources. Missing prerequisites, conflicting identities,
cycles, invalid plans and failed reads abort preparation without mutation.
This requires read access to prerequisite resources, in addition to apply's
ordinary read/create/patch permissions.

`Batch.Warnings()` exposes preparation warnings before writes. `Batch.Execute`
attempts each operation once, skips dependents of failed/skipped operations,
and continues independent operations. Cancellation skips remaining resources.
Results follow execution order and carry `created`, `configured`, `unchanged`,
`failed` or `skipped` status. Resource payloads are sanitized for output. Any
failure or skip returns `ErrBatchFailed` (and cancellation remains inspectable
with `errors.Is`). Batch instances are single-use, including after cancellation.

There is no rollback or transactional guarantee. A failed write may have been
applied if the response was lost or invalid; it is never automatically retried.
Planning reads form a snapshot and do not prevent concurrent changes. Stronger
conditional writes and connector draft lifecycle handling belong to the next
implementation stage. Namespaced-name references remain references in the
request; the server resolves them after ordered prerequisites succeed.

The CLI registers the existing config/signing/service flags and adds
`--request-timeout` (30 seconds per cluster request, 0 to disable). Structured
execution output is an array of result envelopes for JSON or one envelope per
YAML document; client dry-run continues to print desired resources. Name output
prints successful resource identifiers; failures and skips go to stderr.
Warnings are printed before writes, so warning-output errors abort execution.
Result output is buffered until execution completes: a subsequent output error
returns nonzero but cannot undo successful writes or trigger retries. Usage text
is suppressed on errors to keep structured stdout parseable.
