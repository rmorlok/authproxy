# Bounded JavaScript runtime primitives

This package compiles and evaluates pure expressions and async JavaScript
programs in fresh Goja VMs. It owns the
execution deadline, stack limit, interrupt watcher, JSON conversion, and Promise
rejection tracking. Adapters supply native input and optional host capabilities;
the runtime initializes the program, calls it, and waits for its result.

`CompileExpression` prepares one strict expression without running it. The
compiled expression can serve concurrent calls, each with its own VM. For
example, copy `{"price":6,"quantity":7}` into `params` and receive `{"total":42}`:

```go
limits := runtime.Limits{
    MaxSourceBytes: 64 << 10,
    MaxInputBytes: 64 << 10,
    MaxOutputBytes: 64 << 10,
    MaxDepth: 32,
    MaxNodes: 4096,
    MaxStackDepth: 128,
    Timeout: time.Second,
}
expression, err := runtime.CompileExpression(
    `({total: params.price * params.quantity})`, limits,
)
if err != nil {
    return nil, err
}
input := json.RawMessage(`{"price":6,"quantity":7}`)
return expression.Evaluate(ctx, func(runCtx context.Context, vm *goja.Runtime) error {
    decoder, err := runtime.NewJSONDecoder(vm)
    if err != nil {
        return err
    }
    values, err := decoder.Decode(runCtx, runtime.JSONLimits{
        MaxBytes: limits.MaxInputBytes,
        MaxDepth: limits.MaxDepth,
        MaxNodes: limits.MaxNodes,
    }, input)
    if err != nil {
        return err
    }
    if err := vm.Set("params", values[0]); err != nil {
        return runtime.ErrInvalidInput
    }
    return nil
})
```

Here `runtime` refers to `internal/apjs/runtime`, not Go's standard-library
package. Compilation enforces the source-byte cap, rejects statements that
escape the single-expression wrapper, and disables source-map loading. Dynamic
parsing through `eval` or `Function` also has source-map loading disabled.

The preparation callback is trusted integration code. It binds copied JSON and
any selected pure helpers on the calling goroutine, using the same limits passed
to compilation. It must honor `runCtx`, return safe errors, and retain no VM
values. Input preparation, expression execution, and result conversion all use
the same deadline. A preparation error prevents the expression from running.
Pass `nil` for an expression that needs no input. Callers receive detached
`json.RawMessage` bytes and no partial result on failure.

Expressions have no network hosts, connection lookup, or Tool result helpers;
preparation callbacks must preserve that pure profile. Returning a Promise is
an unsupported JSON result, even if it has already settled. Domain adapters
choose bindings such as `params`, `response`, or aliases that share copied data.

`EvaluateJSON` is the lower-level entry point for adapters that own program
execution. Its callback returns a Goja value directly to the private output
encoder. Use `Run` for bounded work with no output, such as initialization
validation. These callbacks also own their input/source bounds and host setup;
they must keep VM values inside the callback.

## Async compilation and initialization

`CompileAsync` requires one top-level `async function execute(...)` declaration.
It checks source without executing it. `Validate` then runs only initialization
in a fresh bounded VM, without invocation inputs or I/O. For example:

```go
program, err := runtime.CompileAsync(`
    const prefix = "item:";
    async function execute(params, context) {
        return prefix + params.id;
    }
`, nil, []runtime.HostBinding{
    {Name: "fetch"},
    {Name: "params", PreserveEnumerable: true},
    {Name: "context", PreserveEnumerable: true},
}, limits)
if err != nil {
    return err
}
return program.Validate(ctx)
```

This passes validation because `params.id` is read only when `execute` is called.
Moving that read to top-level initialization would fail validation. Declaring
`let fetch` would fail compilation because this caller reserved that global.

The caller supplies the complete host binding policy, including names for
capabilities that are disabled. The compiler copies the descriptors and rejects
invalid, duplicate, or escaped names. `execute` belongs to the program and cannot
be a host binding. Initialization makes each declared global nonwritable and
nonconfigurable; absent values become `undefined`. Host bindings are normally
non-enumerable. `PreserveEnumerable` retains an existing JSON alias's visibility
without changing its identity. The Tool adapter supplies its existing host names
and aliases through this same mechanism.

The optional `CompiledLibrary` exposes `Source()` and `Run(vm)`. Its immutable
source must describe the code it runs, and it must support concurrent use in
distinct VMs. Combined program and library bytes share the source allowance.
Helpers keep their existing compilation mode; the program itself is strict.
Library declarations cannot claim host names or `execute`. Library errors are
sanitized before returning to the caller.

`Initialize(vm)` is a low-level integration point. It initializes helpers before
the program, then checks that `execute` is still async. It returns a callable
owned by that VM. Its caller must keep the runtime deadline active and gate I/O
until initialization and its Promise microtasks have completed successfully.
Use it inside `Run` or `EvaluateJSON`; retain no callable or value after cleanup.

`PromiseTracker` records unhandled rejections without retaining private reasons.
Install one before authored initialization and defer `Close`. Call
`CheckRejections` after initialization, and `SettledResult` after the host has
driven the root Promise to settlement or determined it cannot progress. Pending
or rejected roots and unrelated unhandled rejections fail safely. The returned
value stays inside the owning runtime for bounded conversion. `Validate` and
`Evaluate` manage tracking themselves; validation never calls the entry point.

## Evaluating an async program

`AsyncProgram.Evaluate` calls `execute(params, context)` and returns the fulfilled
value as bounded JSON. The preparation callback supplies both arguments. For the
program above, passing `{"id":"42"}` produces the JSON string `"item:42"`:

```go
return program.Evaluate(ctx, func(
    runCtx context.Context, vm *goja.Runtime, invocation *runtime.AsyncInvocation,
) error {
    decoder, err := runtime.NewJSONDecoder(vm)
    if err != nil {
        return err
    }
    values, err := decoder.Decode(runCtx, runtime.JSONLimits{
        MaxBytes: limits.MaxInputBytes,
        MaxDepth: limits.MaxDepth,
        MaxNodes: limits.MaxNodes,
    }, json.RawMessage(`{"id":"42"}`), json.RawMessage(`{}`))
    if err != nil {
        return err
    }
    invocation.Params = values[0]
    invocation.Context = values[1]
    return nil
})
```

The runtime adds no global aliases or context fields. An adapter can bind aliases
to the same native values during preparation. Use `goja.Undefined()` for an
argument that is not needed. Preparation is required and must bound all copied
input, honor `runCtx`, and keep VM values inside this evaluation.

An adapter that supplies asynchronous hosts also fills three optional callbacks:

- `Start` enables host I/O after initialization and its Promise jobs succeed.
  Bound host functions must refuse I/O before this point.
- `Await` waits for and applies one host completion on the VM goroutine. Workers
  exchange detached Go data; only this callback settles their JavaScript Promises.
  It must honor `runCtx` and return `ErrEvaluation` if no work can progress.
- `Close` cancels outstanding work on every exit, including preparation errors
  and panics. Register it immediately after acquiring resources. It runs before
  result encoding and must not wait for uncooperative workers or run authored code.

These callbacks are trusted Go integration code. An evaluation without them can
use `await` and Promise chains, but a root that remains pending fails promptly.
With a host, the runtime calls `Await` only while the root is pending. A winning
`Promise.race` ends execution and cancels unfinished siblings without applying
their late completions. Unhandled rejections are checked at initialization and
final settlement, allowing handlers installed after an awaited operation.

Preparation, initialization, invocation, host completion, cleanup, and output
encoding share one deadline. Callers receive only detached JSON; failure or
cancellation discards the result. Hosts return safe errors unchanged, while
authored exceptions and rejection values remain private.

## Copying JSON into the runtime

Create a `JSONDecoder` before authored code or libraries run. It captures the
original `JSON.parse`, so later changes to that global cannot replace decoding.
Keep the decoder and every returned Goja value inside the owning runtime's
callback, on its goroutine.

Pass all initial documents to one `Decode` call. For example,
`decoder.Decode(runCtx, jsonLimits, paramsJSON, contextJSON)` shares the byte and
node caps across both documents. Each root has depth zero; values and object
property names count as nodes. Serialized bytes include whitespace and escapes.
The whole batch is validated before parsing, and failures return no partial
values. Input byte slices must stay unchanged until the call returns.

The decoder accepts objects, arrays, strings, numbers, booleans, and null. It
creates native JavaScript data with no reflected Go maps or slices. Mutating the
result does not change the input bytes. Empty or nil documents are invalid;
adapters choose defaults and enforce domain shapes such as object-only params.
Numbers use ordinary JavaScript precision. Invalid UTF-8, unpaired surrogate
escapes, and numbers that overflow to infinity are rejected.

Each `Decode` call starts fresh counters. HTTP response readers use this for
their per-body depth/node allowance after the HTTP host has bounded the body
bytes. They set `MaxBytes` to the accepted body's length, keeping HTTP body
accounting separate from `Limits.MaxInputBytes`. Decoding still uses `runCtx`
under the same execution deadline.

## Limits and errors

All limit fields must be positive. `CompileExpression` and `CompileAsync` enforce
source bytes; adapters that compile other program types enforce their own cap. All
input adapters submit their complete batch with the effective JSON limits. The
decoder enforces those byte/depth/node caps. The runtime enforces timeout, stack
depth, and the final output's byte/depth/node limits. Input and output have
separate traversal allowances; a later domain envelope needs its own byte cap.
These are not heap or JavaScript instruction quotas, and native built-ins cannot
always be interrupted immediately.

The timeout covers preparation, authored execution, and output conversion. A
caller cancellation or deadline retains its context error; policy expiry returns
`ErrLimitExceeded`.
The interrupt watcher is joined and cancellation checked again before a result
is returned. `Run` does not hide unrelated Go panics.

Callbacks return safe host errors unchanged. Use `EvaluationError` for errors
from authored compilation/evaluation: it removes private diagnostics and
classifies stack exhaustion as a limit failure. It must not be used on an
already-safe domain error whose classification should survive.

The encoder preserves JSON value kinds, JavaScript number semantics, and the
existing UTF-16-to-JSON behavior. It rejects unsupported values, cycles,
accessors, proxies, sparse arrays, and non-finite numbers. Reflection intrinsics
are captured before the callback, so authored changes to `Object`, `Reflect`,
or `JSON` cannot replace conversion. Getters and `toJSON` are not called.

## Package boundary

This package depends on Goja and the standard library. It has no imports from
root `apjs`, `tools`, core, HTTP integration, or schema packages. Domain adapters
choose input names, capabilities, authorization, and result interpretation.
The existing root `apjs` connector APIs retain their current behavior until
their callers are deliberately migrated to explicit budgets and cancellation.
