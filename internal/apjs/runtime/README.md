# Bounded JavaScript runtime primitives

This package runs trusted Go adapters in a fresh Goja VM and converts their
results into bounded JSON. It owns the execution deadline, stack limit,
interrupt watcher, and output encoder. Tool expression and async evaluation
already use these primitives through their own adapters.

`EvaluateJSON` keeps conversion inside the runtime's deadline. For example, an
adapter can evaluate a small expression and receive `{"total":42}`:

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
source := `({total: 6 * 7})`
if len(source) > limits.MaxSourceBytes {
    return nil, runtime.ErrLimitExceeded
}
return runtime.EvaluateJSON(ctx, limits, func(runCtx context.Context, vm *goja.Runtime) (goja.Value, error) {
    value, err := vm.RunString(source)
    return value, runtime.EvaluationError(err)
})
```

Here `runtime` refers to `internal/apjs/runtime`, not Go's standard-library
package. Dynamic parsing in this VM has source-map loading disabled. An adapter
can instead reuse an already compiled program, provided its compiler applies
the source-size and no-source-map policies.

The callback is trusted integration code. It installs bounded native input and
runs authored JavaScript on the calling goroutine. It must honor `runCtx` during
Go work and must not retain the VM or its values. The callback's returned value
goes directly to the private encoder; the caller receives detached
`json.RawMessage` bytes and no partial result on failure. Concurrent evaluations
use independent VMs.

Use `Run` for bounded work with no output, such as initialization validation.
Neither entry point installs network access, connection objects, Tool helpers,
or domain context. Source compilation, input binding, and async host behavior
remain caller responsibilities in this foundation slice. Shared input/expression
and async APIs will be extracted separately.

## Limits and errors

All limit fields must be positive. The compiling/input adapters enforce source
bytes, input bytes, and input traversal limits. This package enforces timeout,
stack depth, and the final output's byte/depth/node limits. Input and output have
separate traversal allowances; a later domain envelope needs its own byte cap.
These are not heap or JavaScript instruction quotas, and native built-ins cannot
always be interrupted immediately.

The timeout covers the callback and output conversion. A caller cancellation or
deadline retains its context error; policy expiry returns `ErrLimitExceeded`.
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
