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

## Probe policy and migration status

Probe `if.javascript` conditions use the bounded adapter with the calling
request/task context. Each condition has this explicit policy:

| Limit | Value |
| --- | --- |
| Library plus predicate source | 256 KiB |
| Serialized input globals | 1 MiB |
| Boolean JSON output | 5 bytes |
| Value depth | 64 |
| Values and object keys | 50,000 |
| JavaScript call stack | 256 |
| Evaluation timeout | 1 second |

The source check happens before compiling a connector library. A predicate
failure prevents the probe from running and returns a safe error with the probe
ID. Probe HTTP calls keep their existing separate policy. Multiple conditions
get separate execution budgets, while their common caller context can cancel
the whole operation.

The legacy `EvaluateBoolean`, `EvaluateObject`, `TransformOptions`, and
`TransformJSON` APIs remain available during migration. Setup and OAuth scope
predicates, schema validation, option transforms, and migration hooks still use
those APIs. They do not yet gain the bounded adapter's cancellation or copied
inputs. Object and option conversion semantics will be preserved as those
callers move; this slice changes only runtime probe predicates.
