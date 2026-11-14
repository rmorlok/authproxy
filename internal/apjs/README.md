# JavaScript for connectors and shared execution

`apjs` compiles connector helper libraries and evaluates predicates and data
projections. A library can define functions that several connector features use:

```javascript
function isEuropeanTenant() {
    return cfg.region === "eu";
}
```

A predicate such as `isEuropeanTenant() && labels.env === "prod"` reads the
connection's configuration and metadata. Each evaluation gets a fresh VM, so
library globals do not carry state between calls. Library initialization runs
before input globals are installed: helpers can read `cfg` when called, but
top-level code cannot use it during initialization.

The shared execution packages are:

- [`runtime`](runtime/README.md): bounded VM lifecycle, native JSON copying,
  pure expressions, async execution, connection bindings, and result extensions.
- [`http`](http/README.md): HTTP budgets, response ownership, and send evidence
  shared by callers that supply their own clients.

## Bounded connector predicates

Call `Context.EvaluateBooleanWithContext` with the real request or task context
and explicit `runtime.Limits`:

```go
jsctx := apjs.NewContext(library, map[string]any{
    "cfg":         map[string]any{"region": "eu"},
    "labels":      map[string]string{"env": "prod"},
    "annotations": map[string]string{},
})
enabled, err := jsctx.EvaluateBooleanWithContext(
    ctx, `isEuropeanTenant() && labels.env === "prod"`, limits,
)
```

All limit fields must be positive. Expression and library source share the byte
cap; callers must check their combined size **before compiling the library**,
including before a library-cache lookup that may compile it. The method checks
that same allowance before parsing the predicate. Its deadline covers predicate
compilation, input copying, library initialization, evaluation, and result
conversion. Go parsing and scalar serialization are bounded by input size and
checked for cancellation around those operations; they are not interruptible Go
work. Caller cancellation takes precedence over the execution timeout.

This adapter preserves connector predicate syntax: non-strict JavaScript,
statement lists, and trailing semicolons are accepted. For example:

```javascript
var enabled = isEuropeanTenant();
enabled;
```

The final result must be a boolean. JavaScript `Boolean` wrappers retain their
legacy meaning, so `new Boolean(false)` returns false. Other objects are rejected
without coercion, getter access, or exporting an arbitrary object graph.

Inputs are copied into native JavaScript values. Supported Go data is `nil`,
bools, strings, built-in integer/float types (excluding `uintptr`),
`json.Number`, `map[string]any`, `map[string]string`, `[]any`, and `[]string`.
Structs, pointers, other named types, custom marshalers, and live Go values are
rejected. Missing variable maps become `{}`; nested nil maps/slices become null.
The complete variables object shares the byte, node, and depth budgets, with
its root at depth zero. Conversion checks limits before growing its output,
and never invokes custom marshaling methods. Callers must keep their Go data
stable during evaluation.

A predicate can change its local `cfg` or labels, but those edits do not change
connection data or another evaluation. No HTTP, module, filesystem, environment,
or subprocess host is installed. Source maps are disabled for static and dynamic
parsing. Libraries retain the existing reserved-name checks for `cfg`, `labels`,
`annotations`, and `data`.

Failures return false and fixed shared-runtime errors. Errors do not include
source, thrown values, private input, or parser diagnostics. Context cancellation
retains its context error, while policy expiration returns `ErrLimitExceeded`.
These bounds limit execution and host conversion work; they are not a process
sandbox or a hard heap quota.

## Bounded connector validation

`CompileAndValidateLibraryWithContext` checks raw source size before parsing and
runs helper initialization in a fresh bounded VM:

```go
library, err := apjs.CompileAndValidateLibraryWithContext(
    ctx, 
	`function isEnabled() { return cfg.enabled === true; }`,
    apjs.ConnectorPredicateLimits(),
)
```

Initialization has no input globals or HTTP hosts. The function above is valid
because it reads `cfg` only when called. Top-level `const enabled = cfg.enabled`
fails validation because `cfg` is not available yet. Reserved-name checks run
under the same deadline, including getters installed by the library. Parsing is
byte-bounded and checked for cancellation around the call, but cannot be
interrupted midway. Successful libraries retain their raw source for subsequent
combined library/predicate limits, including whitespace-only source.

Library validation errors include the first syntax error's one-based line and
column within the library source when available, or the conflicting reserved
global name (`cfg`, `labels`, `annotations`, or `data`). They still match
`ErrInvalidSource` and do not retain parser messages, source text, filenames, or
thrown values. Runtime failures and
predicate evaluation errors remain opaque shared-runtime categories.

Connector schema validation uses this API for the library, then evaluates each
Boolean predicate with empty `cfg`, `labels`, and `annotations` objects. A false
result is valid; a non-Boolean result is rejected. This preserves the existing
validation contract rather than predicting the result for a real connection.

`common.ValidationContext` carries cancellation through nested fields. API
resource binding/rendering and connector create/update operations supply their
request context. Synchronous callers that omit it, such as configuration and
local manifest validation, still get finite execution limits. Each library and
predicate gets a separate budget; there is no total connector-validation timeout
unless the caller supplies one. Schema errors keep field paths and safe failure
text; these existing wrappers stringify the underlying error rather than retain
its Go error identity. The bounded library API itself preserves context errors.

## Connection predicate policy and migration status

`iface.Connection.EvaluateJavascriptPredicate(ctx, source)` loads the captured
connector library and connection data, then calls the bounded adapter. Probes,
setup-step eligibility, and OAuth scope inclusion/required predicates use this
method with their request/task context. `ConnectorPredicateLimits()` supplies
their shared explicit policy, also used for library and Boolean predicate schema
validation; it returns a fresh value on each call:

| Limit | Value |
| --- | --- |
| Library plus predicate source | 256 KiB |
| Serialized input globals | 1 MiB |
| Boolean JSON output | 5 bytes |
| Value depth | 64 |
| Values and object keys | 50,000 |
| JavaScript call stack | 256 |
| Evaluation timeout | 1 second |

Definition loading observes the caller context and preserves decryption/decode
errors for retry decisions. The source check happens before compiling a connector
library; only authored compile failures become `ErrInvalidSource`. A predicate
failure stops the operation with its probe, step, or scope identity in the error.
HTTP calls keep their existing separate policy. Multiple conditions get separate
execution budgets, while their common caller context can cancel evaluation.

Setup form and redirect steps evaluate their conditions when the flow checks
eligibility. OAuth evaluates inclusion before dynamic required status and returns
cloned scope declarations with concrete Boolean requirements. Constants and
omitted conditions need no JavaScript execution. Each condition gets a fresh VM;
mutating `cfg` in one condition cannot affect a later step or scope decision.
Setup flow construction remains separate from predicate evaluation; its existing
definition lookup is not changed by this migration.

## Bounded migration hooks

`Context.EvaluateObjectJSONWithContext` evaluates a connector expression and
returns detached JSON object bytes. Boolean predicates and object expressions
share input copying, helper initialization, cancellation, and bounded output
encoding. The returned bytes can be decoded directly into a caller's patch type.

For example, an upgrade hook can rename a saved configuration field:

```javascript
({
    config: {
        set: { workspaceId: cfg.workspace_id },
        unset: ["workspace_id"]
    }
})
```

Migration hooks receive the candidate's `cfg`, user `labels`, and `annotations`.
The selected generation's helper library is available to both upgrade and
downgrade hooks. Only the returned patch changes the candidate: assigning to
`cfg` inside JavaScript edits a private copy. The next generation sees changes
from the previous generation's successfully applied patch.

Root `null` and `undefined` retain their existing meaning: an empty `{}` patch.
Other results must be plain JSON objects. Arrays and scalar roots are rejected.
Object properties valued `undefined` are omitted at any depth, matching
`JSON.stringify`. In the example above, if `cfg.workspace_id` is missing, `set`
becomes `{}` and the hook succeeds without setting `workspaceId`. Explicit `null`
is retained, so `workspaceId: null` sets a JSON null value.

Nested getters, proxies, custom prototypes, cycles, functions, undefined array
elements, and non-finite numbers are rejected by the shared encoder. This
deliberately narrows the legacy object export behavior; migration hooks should
return ordinary JSON data rather than rely on getter execution or implicit
object conversion.
Property omission is an explicit object-evaluator option; other runtime callers
keep the strict default. It does not invoke getters or `toJSON`, and inspecting
omitted properties still counts toward the traversal budget.

`ConnectorMigrationLimits()` uses the source, input, depth, node, stack, and
timeout limits above, with a **1 MiB JSON output allowance**. Each generation's
hook gets its own JavaScript/encoding budget under the common migration context.
Definition loading uses that context, and raw library-plus-hook source is checked
by the shared bounded library loader before compilation or reuse from its cache.
The loader also serves connector predicates and checks each caller's combined
source allowance, including callers with a tighter limit than the first compile.
JavaScript and encoding errors are opaque; definition-load failures retain their
causes. Typed patch decoding and existing patch validation follow evaluation,
with caller cancellation checked before applying the patch.

Hook compilation and evaluation failures write an application log with the
connection and generation IDs, the failure category, and the evaluation phase
(such as `execute` or `output`).
Syntax errors and throws also include the `library` or `expression` source unit
and one-based line/column when available. For example, a helper that throws can
report `phase=execute source=library line=7 column=3` while the API error stays
`JavaScript execution failed`. This metadata is available to trusted adapters
through `Diagnostic(err)`. Logs omit exception messages, source snippets,
configuration, returned values, and JSON property names, which can all contain
private data. Output rejections currently identify the phase and category,
without a JSON path.

The unused unbounded object evaluator has been removed; object callers use
`EvaluateObjectJSONWithContext`. Other legacy library and expression APIs remain
available during migration.
Option transforms still use unbounded execution and their existing conversion
semantics. Migration-hook syntax validation remains on its legacy compile-only
path; it does not execute the hook and is a separate migration follow-up.
