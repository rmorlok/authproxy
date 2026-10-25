# Pure Tool expressions

This package compiles a JavaScript expression once and evaluates it against a
captured Tool invocation and prepared response data. For example, an HTTP Tool
can turn a provider's customer record into a smaller object:

```javascript
({ customerId: response.body.id, tenant: context.cfg.tenant })
```

Each call uses a fresh Goja runtime. Compilation checks syntax without executing
the expression. Evaluation provides native JavaScript copies of the input and
returns a bounded JSON value. It does not send HTTP requests or produce the
complete public `ToolResult` envelope.

## Example

```go
program, err := javascript.CompileExpression(
    `({ customerId: response.body.id, tenant: context.cfg.tenant })`,
    javascript.Limits{
        MaxSourceBytes: 4096,
        MaxInputBytes:  64 * 1024,
        MaxOutputBytes: 16 * 1024,
        MaxDepth:      32,
        MaxNodes:      4096,
        MaxStackDepth: 128,
        Timeout:       100 * time.Millisecond,
    },
)
if err != nil {
    return err
}

snapshot, err := render.NewContext(render.Input{
    Params:       map[string]any{"customerId": "customer-42"},
    Cfg:          map[string]any{"tenant": "acme"},
    ConnectionID: "cxn_01example0000001",
}, render.Limits{MaxBytes: 64 * 1024, MaxDepth: 32, MaxSteps: 4096})
if err != nil {
    return err
}

result, err := program.Evaluate(ctx, javascript.Input{
    Context: snapshot,
    Response: common.RawJSON(`{
        "status": 200,
        "headers": {"content-type": "application/json"},
        "mediaType": "application/json",
        "body": {"id": "customer-42", "internalNotes": "..."}
    }`),
})
```

The result is JSON equivalent to
`{"customerId":"customer-42","tenant":"acme"}`. `common`, `render`, and
`javascript` refer to the packages under `internal/schema/common`,
`internal/tools/render`, and `internal/tools/javascript`.

The caller validates Tool arguments and captures safe connection configuration
before creating the snapshot. The HTTP executor prepares the response object;
this package does not interpret Content-Type, read or close streams, or decide
whether an HTTP status is an error. A missing `Response` becomes `{}`. A supplied
response must be a JSON object, though its fields can contain any supported JSON
values, including an explicit null body.

Input must be valid UTF-8. Unpaired surrogate escapes in JSON strings or keys
are rejected because Goja's JSON parser would replace them with a different
character. Valid surrogate pairs and literal backslash text are accepted.

## Available data

| Expression name | Value |
| --- | --- |
| `params` | Validated invocation arguments from the snapshot. |
| `context.cfg` | Captured connection configuration, such as tenant and API host. |
| `context.labels`, `context.annotations` | Captured connection metadata. |
| `context.connection` | Safe connection identity fields. |
| `context.tool` | Captured Tool identity and revision metadata. |
| `response` | The caller-prepared response object. |
| `cfg`, `labels`, `annotations` | Aliases of the corresponding objects in `context`. |

The six injected global bindings cannot be replaced. Their nested objects are
ordinary JavaScript objects: an expression may change its private copy, but the
change never persists to the snapshot or another evaluation. Compiled programs
and immutable snapshots may be reused concurrently. No Go maps, structs,
methods, connection handles, or credentials are reflected into the runtime.

Only standard Goja language facilities are available. There is no `fetch`,
timer, module loader, filesystem, process environment, or application logger.
The [HTTP response mapper](../httpcodec/README.md#response-transforms) uses this
runtime for `transformJavascript`, preparing the response object and converting
the projection into a rich result. Asynchronous Tool entry points, connector
helper libraries, `ToolError`, and `toolResult` remain separate work.

## Results and errors

Results can be null, booleans, finite numbers, strings, dense arrays, or ordinary
objects containing those values. Arrays and objects retain their shape. A
string result is JSON-quoted; conversion into a rich text block belongs to the
Tool result layer.

Conversion rejects undefined, functions, symbols, BigInts, non-finite numbers,
cycles, array holes, extra array properties, accessors, proxies, and exotic
objects such as dates, promises, maps, sets, and typed arrays. It reads ordinary
data properties without invoking getters or `toJSON`. Unsupported values are
errors rather than being silently omitted or converted to null. Object output
uses own enumerable string properties; ordinary non-enumerable data is omitted.

Goja does not expose a distinct public type for every built-in object. If author
code deliberately gives an otherwise indistinguishable object, such as a
WeakMap, a plain or null prototype, conversion projects only its own JSON data
properties. It does not serialize hidden collection or iterator state.

Numbers use JavaScript Number semantics. For example, parsing
`9007199254740993` rounds it to `9007199254740992`; transformations do not retain
raw numeric spelling. Input numbers outside the finite Number range are
rejected. The HTTP codec's default JSON passthrough remains the path for
preserving original JSON number tokens without JavaScript evaluation.

Supply identifiers that need exact digits as JSON strings before they enter the
runtime. For example, a response body of `{"id":"9007199254740993"}` keeps the
identifier unchanged in this projection:

```javascript
({ customerId: response.body.id }) // {"customerId":"9007199254740993"}
```

This also applies to identifiers in `params` and connection configuration.
Calling `String(response.body.id)` after a numeric ID has been parsed cannot
recover digits that were already rounded.

Failures return no partial output. Fixed error classifications distinguish
invalid source, input, output, execution limits, and thrown JavaScript errors.
Errors never contain expression source, thrown values, stack traces, private
configuration, or response data. Caller cancellation and deadlines remain Go
context errors. The configured timeout returns `ErrLimitExceeded`, allowing an
executor to report an execution-limit failure separately from a canceled
caller request. If both contexts have expired, the caller's error wins.

## Limits and lifecycle

Every limit must be positive. The caller resolves operator ceilings before
compilation; this package supplies no unlimited defaults.

- `MaxSourceBytes` bounds authored UTF-8 source before parsing.
- `MaxInputBytes` bounds the serialized snapshot and response together.
- `MaxOutputBytes` bounds emitted JSON, including punctuation and escaping.
- `MaxDepth` bounds nested JSON values, with the root at depth zero.
- `MaxNodes` bounds JSON values and object property inspections. Combined input
  and output each receive this allowance; it is not a JavaScript instruction count.
- `MaxStackDepth` bounds JavaScript function-call recursion.
- `Timeout` covers binding input, evaluating the expression, and converting its
  result. An earlier caller deadline wins.

A watcher interrupts busy JavaScript loops on cancellation and is joined before
evaluation returns. It uses only Goja's thread-safe interrupt method; all other
runtime work stays on one goroutine. Fresh VMs prevent a late interrupt or a
modified global from affecting the next call.

These limits bound processing for trusted administrator code. They do not make
the in-process runtime a hard memory-isolated sandbox. Native Goja built-ins
cannot be preempted by an interrupt, and JavaScript can allocate memory before
output conversion. The caller also owns authorization, aggregate invocation
accounting, output-schema validation, and limits on the final rich-result envelope.
