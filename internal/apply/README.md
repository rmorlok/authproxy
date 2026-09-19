# Apply infrastructure

The apply implementation runs in stages:

* **Load** - read resource files and validate
* **Resolve** - read from the cluster to resolve targets and references, and
  create plans for creates/updates
* **Apply** - execute the plan with creates/updates

When planning updates, the system computes the patch that is needed to apply the
changes rather than setting the state of the resources completely. It tracks the
previously applied state via the `authproxy.net/last-applied-configuration`
annotation, and compares that history with the live state and new desired state
to decide which attributes should be created/updated/deleted. This allows it to
preserve fields that apply does not manage, such as labels or annotations added
by separate processes. Only labels/annotations defined by apply would be updated
by apply, apart from its reserved history annotation. Changes to managed fields
are restored by default or reported as conflicts with `--overwrite=false`.

Much of the logic in this package is centered around ingesting the JSON/YAML
that is being supplied for the apply and then validating fields and redacting
data that may be secret. It uses the JSON schema definitions for the resources
to establish which fields are allowed, and it leverages annotations on the
canonical resource definition structs to determine which fields are secret. To
accomplish this it extracts field paths that it can apply to the normalized
document. Secret presence in history is recorded using JSON pointers, such as
`/spec/keyData`.

Secret redaction is especially important to this package because it stores the
previous apply configuration in `authproxy.net/last-applied-configuration` and
if a field were a secret value, it would be directly visible there. Instead it
excludes secret values before storing history in that annotation and records
only their presence. It then handles those presence markers when computing a
plan; masked secret values are not stored in history.

## Key Terminology

* **Live** - refers to the resource definition that is downloaded from the
  cluster as the apply is being computed.
* **Document** - the normalized input loaded from JSON/YAML for the apply.
  This includes the location where the document was taken from, including
  the fact that there can be multiple resources in a single file. The
  document retains much of the raw object structure so it can be validated
  while preserving omitted, null and empty fields. It explicitly extracts things
  like the resource kind and common metadata block
  (name/id/namespace/labels/annotations).
* **History** - the previous desired state, with secrets excluded and their
  presence recorded, stored in `authproxy.net/last-applied-configuration`. Its
  version identifies the history format, not the resource generation.
* **Target** - a document paired with its current live resource, or no live
  resource when a name lookup permits creation.
* **Reconciliation** - the three-way comparison of history, live state and new
  desired state that produces a plan.
* **Managed field** - a field tracked in the previous desired state or supplied
  in the new desired state. Omission of a previously managed field can remove it;
  omission of a secret preserves it.
* **Plan** - a computed operation for one resource: create, update or unchanged.
  An update can contain mutations to several fields; a plan is not yet executed.
* **Batch** - a single-use collection of validated plans ordered by their
  dependencies. A prerequisite must succeed before its dependent runs.
* **Result** - the outcome of a batch operation, including its status, a sanitized
  resource when available, and an error for a failure or skip.

## Details

The CLI defaults to cluster execution. `--dry-run=client` stops after loading
and prints sanitized desired resources; it does not read cluster state or
compute a server-backed plan. The normal path is `Load` → `Client.Prepare` →
`Batch.Execute`. Preparation includes resolution, reconciliation and dependency
checks before any writes.

### Loading and validation

`Load` turns manifests into normalized documents. It is offline apart from
explicit manifest URL downloads. Each document retains both its field presence
and its canonical typed resource, so validation and later comparisons can
distinguish omission from explicit null or empty values.

`Load` accepts `ValidationStrict` (default), `ValidationWarn`, or
`ValidationIgnore`. Warn/ignore remove unknown fields using the registry's
embedded resource schema before strict typed decoding. Warn reports counts and
source locations through `Options.Warn`. Arbitrary map entries remain intact.
Malformed input, duplicate keys, invalid identity, server-owned fields, wrong
types and redacted placeholders always fail. The CLI exposes these as
`--validate=strict|warn|ignore`; `--overwrite` is accepted but has no effect on
client dry-run.

Resource recognition, patch factories, metadata access, ID validation and
canonical lifecycle/merge capabilities come from `schema/registry`. Apply owns
manifest loading, transport, target resolution, reconciliation and operation
policy. The registry supplies the same resource contracts used elsewhere in the
application.

### Connecting and resolving targets

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

`ResolveBatch` returns targets, not retained execution plans. `Client.Prepare`
uses `Reconcile` on each target with the caller's overwrite policy to obtain the
plans it will execute.

`LiveResource` retains typed responses, the redaction header, and known
write-only field paths. Never derive mutations from masked values or assume
omitted write-only fields are absent on the server. Resources and documents may
contain secrets and must not be logged. API errors expose HTTP status without
reflecting response bodies, and the HTTP client refuses redirects so signing
credentials are not forwarded elsewhere.

### Computing a plan

`Reconcile(target, ReconcileOptions{Overwrite: true})` returns a validated
`Plan` with `create`, `update`, or `unchanged` operation and warnings. It does
not contact the cluster or mutate inputs. For create, submit `plan.Document`
to `Client.Create`; for update, submit `plan.Target` and `plan.Patch` to
`Client.Update`. Unchanged plans need no write. Batch execution uses these
plans; client dry-run does not resolve live state or run reconciliation.

For an existing resource, history identifies the fields previously supplied by
apply. Reconciliation compares those values with live and new desired values to
preserve unmanaged fields and calculate changes to managed fields. For example,
if apply previously set `labels.team` and another process added `labels.audit`,
a manifest changing `team` updates that key and retains `audit`. Omitting `team`
from the next manifest removes it because it was previously managed.

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

### Selecting connector generations

`resolveApplyTarget` keeps ordinary `Resolve` semantics for references, but
selects a reconciliation target appropriate to the logical update endpoint. It
reads all generation pages, validates identities and pagination, and inspects
observed release state. This requires `connectors:list/generations` permission.
An existing draft takes precedence for edits; without one, definition changes
merge against the newest generation the server will clone. A primary declaration
already satisfied by the selected primary does not publish an unrelated draft.

`Reconcile` retains explicit publication intent when sending a changed definition,
even when its source already declares primary. Equal definitions are omitted;
explicit secrets remain writes because their live values cannot be compared.
Explicit generation targets must be drafts for any update, including history
adoption. They are never redirected to another generation or forced into a state.

### Storing history without secrets

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
server-rejected writes do not advance history. A lost response after a successful write
still has an uncertain outcome; there is no second history-only request.

### Ordering the batch

A prerequisite is a resource that another resource needs, such as its namespace
or a key named in an encryption-key reference. A dependency edge records which
operation must succeed first. A cycle means no operation in the cycle can run
first, so preparation rejects it.

`Client.Prepare(ctx, documents, options)` resolves every target and validates
all reconciliation plans before writes. It discovers explicit typed
`meta.ObjectReference` values through `registry.References`, verifies external
references and namespace prerequisites, and orders prerequisites before
dependents. It selects the earliest ready input when multiple resources can run.
Selected input must include prerequisites being created, or those prerequisites must already
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

### Executing and reporting results

`Create` and `Update` submit one validated operation. They use canonical
resource and patch types, including immutable-field checks and
omitted/null/empty patch semantics. `Update` accepts a calculated canonical
patch; callers must perform three-way reconciliation before using it.

`Batch.Warnings()` exposes preparation warnings before writes. `Batch.Execute`
attempts each operation once, skips dependents of failed/skipped operations,
and continues independent operations. Cancellation skips remaining resources.
Results follow execution order and carry `created`, `configured`, `unchanged`,
`failed` or `skipped` status. Resource payloads are sanitized for output. Any
failure or skip returns `ErrBatchFailed` (and cancellation remains inspectable
with `errors.Is`). Batch instances are single-use, including after cancellation.

There is no rollback or transactional guarantee. A failed write may have been
applied if the response was lost or invalid; it is never automatically retried.
`Batch.Execute` refreshes and reconciles each target immediately before dispatch,
including unchanged plans, using the original document and overwrite policy.
Existing targets are pinned to their resolved IDs; a target appearing after a
planned create fails without adoption. Removed history also fails so adoption
can be reviewed in a newly prepared batch. Conflicts and precondition failures are
reported without replaying writes. Callers must prepare a new batch after review.

Refresh covers every batch resource kind and the history annotation in its patch,
but it is not an atomic precondition. There is no ETag/revision enforcement, so a
writer between the final read and mutation can still be overwritten. Some server
updates perform metadata/history and generation writes separately; an error can
therefore follow partial effects. No strong conflict guarantee is claimed, even
with `Overwrite: false`. Low-level `Create`/`Update` still submit once without
refresh; the executor owns refresh and reconciliation. Namespaced-name references remain references in the
request; the server resolves them after ordered prerequisites succeed.

Server validation and authorization remain authoritative.

The CLI registers the existing config/signing/service flags and adds
`--request-timeout` (30 seconds per cluster request, 0 to disable). Structured
execution output is an array of result envelopes for JSON or one envelope per
YAML document; client dry-run continues to print desired resources. Name output
prints successful resource identifiers; failures and skips go to stderr.
Warnings are printed before writes, so warning-output errors abort execution.
Result output is buffered until execution completes: a subsequent output error
returns nonzero but cannot undo successful writes or trigger retries. Usage text
is suppressed on errors to keep structured stdout parseable.
