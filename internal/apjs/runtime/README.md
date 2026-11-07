# Bounded JavaScript runtime primitives

This package compiles pure JavaScript expressions and runs trusted Go adapters
in fresh Goja VMs. It owns the execution deadline, stack limit, interrupt watcher,
JSON input decoder, and bounded output encoder. Tool expressions delegate to the
shared expression type; async Tool evaluation uses the lower-level primitives.

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
they must keep VM values inside the callback. Shared async APIs will be
extracted separately.

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

All limit fields must be positive. `CompileExpression` enforces source bytes;
adapters that compile other program types enforce their own source cap. All
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
