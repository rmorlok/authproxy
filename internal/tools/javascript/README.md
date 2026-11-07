# Tool JavaScript

This package evaluates pure expressions and async JavaScript Tool functions.
It reuses connector helper libraries and bounds initialization, execution, and
JSON conversion with explicit limits. Tools can opt into `fetch` through one
captured connection's HTTP session. Pure expressions and ordinary Tool evaluation
have no I/O capabilities.

The shared [apjs/runtime primitives](../../apjs/runtime/README.md) own the VM
lifecycle, expression evaluation, async compilation and execution, Promise
tracking, execution limits, and bounded native JSON conversion. This package
supplies Tool snapshots and aliases, its reserved host names, connector-library
selection, and the HTTP host's workers and response readers. Its
`Limits` type refers to the shared policy. Callers reference error classifications
directly from `internal/apjs/runtime`.

## Pure expressions

An expression is compiled once and evaluated against a captured Tool invocation
and prepared response data. For example, an HTTP Tool
can turn a provider's customer record into a smaller object:

```javascript
({ customerId: response.body.id, tenant: context.cfg.tenant })
```

Each call uses a fresh Goja runtime. Compilation checks syntax without executing
the expression. Evaluation provides native JavaScript copies of the input and
returns a bounded JSON value. It does not send HTTP requests or produce the
complete public `ToolResult` envelope.

### Example

```go
limits := javascript.Limits{
    MaxSourceBytes: 4096,
    MaxInputBytes:  64 * 1024,
    MaxOutputBytes: 16 * 1024,
    MaxDepth:      32,
    MaxNodes:      4096,
    MaxStackDepth: 128,
    Timeout:       100 * time.Millisecond,
}
program, err := javascript.CompileExpression(
    `({ customerId: response.body.id, tenant: context.cfg.tenant })`,
    limits,
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
the projection into a rich result. Expressions do not receive connector helper
libraries. Tool entry points use the separate compilation path below.

## Tool entry points and helper libraries

A Tool script declares one top-level `async function execute`. It can call
functions from the connector's compiled JavaScript library. For example, a
connector can define a shared way to identify customer records:

```javascript
function customerKey(id) {
    return cfg.tenant + ":" + id;
}
```

A Tool uses that helper with its arguments and captured connection configuration:

```javascript
async function execute(params, context) {
    return { customerKey: customerKey(params.customerId) };
}
```

With `params.customerId` equal to `"customer-42"` and the connection's `tenant` set to
`"acme"`, the returned promise resolves to `{ customerKey: "acme:customer-42" }`.
Parameter names are the author's choice; invocation supplies arguments and
context by position.

Compile and check initialization using the same `limits` policy as above:

```go
library, err := apjs.CompileAndValidateLibrary(`
    function customerKey(id) { return cfg.tenant + ":" + id; }
`)
if err != nil {
    return err
}
tool, err := javascript.CompileTool(`
    async function execute(params, context) {
        return { customerKey: customerKey(params.customerId) };
    }
`, library, limits)
if err != nil {
    return err
}
if err := tool.Validate(ctx); err != nil {
    return err
}
```

`CompileTool` supplies the Tool host policy to the shared compiler, which checks
syntax, the entry-point declaration, reserved names, and combined source size.
It never runs authored code. `Validate` runs library
initialization followed by Tool initialization in a fresh VM with no host
capabilities or connection data. It checks that `execute` is still an async
function after top-level code runs but does not call it; replacing the declaration
with a synchronous function fails. Publication has no `cfg`, `params`, or other
invocation values: helpers must read them inside functions called during
evaluation. For example,
`const prefix = "customer:"` is valid top-level initialization, while
`const tenant = cfg.tenant` fails validation. A top-level `fetch(...)` also fails
because validation has no fetch host. Authored top-level code can itself call
other functions, so initialization is covered by cancellation, timeout, and
stack limits. Unhandled Promise rejections during initialization fail validation;
rejections caught before its Promise jobs finish are allowed. Evaluation checks
this same initialization boundary before calling `execute`, so its body cannot
recover from an initialization rejection.

Tool code uses strict JavaScript. Connector libraries keep their existing syntax
rules. Both reserve global host names such as `fetch`, `context`, `params`,
`ToolError`, and `toolResult`; the library also reserves `execute`. Local helper
variables may reuse these names. Legacy unbraced conditional declarations, such
as `if (ok) function fetch() {}`, are rejected anywhere in a library because
they can reach the global object. Host globals are frozen before initialization,
including absent capabilities whose value is `undefined`. Host capabilities such
as `fetch` are non-enumerable; existing captured input bindings remain enumerable.
Tool, expression, and connector library parsing, including dynamic
`eval`/`Function` parsing, do not load source maps.

`Initialize(vm)` delegates to the shared async initializer. The invocation
caller installs capabilities and copied input, owns a fresh VM on one goroutine,
and keeps a deadline active through initialization and execution. The returned
callable belongs to that VM. Initialization sets the stack cap but does not start
its own timeout. Most callers should use `Evaluate` or `EvaluateWithHTTP` for the
complete bounded lifecycle described below. `ToolError`/`toolResult` helpers and
invocation-service integration remain separate work.

## Evaluate an async Tool

`ToolProgram.Evaluate` binds captured Tool input and delegates invocation and
Promise settlement to the shared runtime. It calls `execute(params, context)` and
converts its fulfilled native Promise result into JSON. The Tool and its called
helpers share a private copy of the captured input. `await` and Promise chains
can compose their results:

```javascript
async function execute(params, context) {
    const key = await Promise.resolve(customerKey(params.customerId));
    return { customerKey: key };
}
```

With the compiled `tool` and captured `snapshot` from the examples above:

```go
result, err := tool.Evaluate(ctx, javascript.Input{Context: snapshot})
if err != nil {
    return err
}
```

The fulfilled value becomes bounded JSON using the same rules as expressions.
`Response` is optional and normally omitted for Tool functions. It is available
for callers that already have prepared response data. Changing `params` or
`context.cfg` changes only that evaluation's copy, including changes made by called
helpers. Reusing a compiled Tool starts fresh each time.

Evaluation fails if `execute` throws or rejects, or another Promise remains
rejected without a handler when work finishes. Rejections that are caught before
then are allowed. A Promise that remains pending with no host work capable of
settling it fails immediately; for example, `await new Promise(() => {})` does
not consume the whole timeout. A busy loop or endless Promise chain is interrupted
by the timeout. Errors contain no thrown values, private input, or stack traces.

Evaluation runs synchronously on one Goja goroutine. Goja drains Promise jobs
when authored calls finish, which supports `await` and Promise chains that can
settle without a network or timer host. All JavaScript execution and Promise
callbacks stay on that goroutine. The shared runtime tracks abandoned rejections
and removes them when a handler is attached. For HTTP evaluation, it enables the
host after initialization, requests one completion at a time while the root is
pending, and cancels outstanding host work before encoding the result.

## Fetch through a connection

`EvaluateWithHTTP` adds asynchronous `fetch` to Tool execution. For example,
this Tool looks up a customer and updates that same customer's record:

```javascript
async function execute(params, context) {
    const base = context.cfg.apiBase;
    const lookup = await fetch(base + "/customers/" + encodeURIComponent(params.id));
    if (!lookup.ok) return { updated: false, status: lookup.status };

    const customer = await lookup.json();
    const updated = await fetch(base + "/customers/" + encodeURIComponent(customer.id), {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: true })
    });
    return { updated: updated.ok, status: updated.status };
}
```

Both requests use the session's captured connection and authentication. The
caller authorizes the Tool and constructs the configured connection client
before evaluation, as shown in [httphost](../httphost/README.md). Given that
`client`, a compiled `tool`, and a captured `snapshot`:

```go
session, err := httphost.New(ctx, client, httphost.Limits{
    Timeout:          30 * time.Second,
    MaxRequests:      8,
    MaxResponseBytes: 2 * 1024 * 1024,
    MaxStreamBytes:   2 * 1024 * 1024,
    MaxHeaderBytes:   32 * 1024,
})
if err != nil {
    return err
}
defer session.Close() // Safe to repeat; evaluation also closes the session.

result, err := tool.EvaluateWithHTTP(ctx, javascript.Input{Context: snapshot}, javascript.FetchOptions{
    Session:              session,
    MaxRequestBytes:      64 * 1024,
    MaxPendingOperations: 4,
    Logger:               logger,
})
state := session.ExecutionState() // Inspect even when evaluation failed.
```

Evaluation takes ownership of the dedicated session and closes it before
returning, including when arguments or options are invalid. It cannot be reused.
`ExecutionState` preserves cumulative send
evidence; cancellation or failure does not undo an earlier request. If a client
ignores cancellation, an outstanding call remains conservatively uncertain.

The supported Fetch subset is explicit:

| Surface | Behavior |
| --- | --- |
| `fetch(url, init)` | Absolute HTTP(S) URL string; optional own `method`, `headers`, and `body` fields. Unsupported fields are rejected. |
| Request method | `DELETE`, `GET`, `HEAD`, `OPTIONS`, `POST`, and `PUT` are normalized to uppercase. `CONNECT`, `TRACE`, and `TRACK` are rejected regardless of case. Other methods retain their spelling. |
| Request headers | Own enumerable string values. `Host` overrides the request host; framing and hop-by-hop headers remain transport-owned. |
| Request body | String or `ArrayBuffer`, copied before dispatch. Serialize JSON with `JSON.stringify` and set its content type explicitly. Typed arrays are not accepted directly. |
| Response metadata | Read-only `status`, `ok`, `bodyUsed`, and `headers.get(name)`. Header lookup is case-insensitive and returns null for a missing or filtered header. |
| Response body | Single-use async `text()`, `json()`, or `arrayBuffer()`. A second admitted read rejects. If worker admission is refused, `bodyUsed` stays false and the read can be retried after other work finishes. |

There are no `Request`/`Headers` constructors, streaming readers, cloning,
per-request abort signals, timers, or URL/text/base64 helpers in this slice.
JSON parsing uses the captured native parser and the runtime's depth/node limits.
Binary reads preserve bytes; text reads decode UTF-8 with replacement for invalid
sequences. Known sensitive response headers are filtered by the HTTP session.

Non-2xx statuses resolve normally so the Tool can inspect them without reading
an unwanted body. Transport and body failures reject with fixed error messages;
private upstream errors, URLs, and JavaScript stacks are never included. Catching
a rejection can recover an individual request failure, but cannot restore an
exhausted session-wide response allowance or expired timeout.

Fetch is disabled during initialization, including Promise jobs created while
checking the entry point. Only `execute` and work it starts may send requests.
When `execute` settles, remaining operations are canceled and their late results
are discarded without running more JavaScript callbacks. This lets a caught
`Promise.all` failure return a fallback, and lets `Promise.race` or `Promise.any`
return a winner without waiting for other requests. Already-observed unhandled
rejections still fail evaluation. Cancellation cannot undo an upstream effect;
inspect `ExecutionState` even when the Tool returns successfully.

Parallel requests share `MaxPendingOperations`; exceeding that cap rejects
before launching a worker. Bodies are read only when a body method is admitted.
`MaxRequestBytes` bounds each prepared request, while the session shares request
count and decoded response allowances across the invocation.

HTTP workers exchange detached Go data with a single VM-owning goroutine.
Cancellation interrupts busy JavaScript and pending I/O. Cleanup initiates body
closure without waiting for an uncooperative transport; late workers cannot
resume a disposed VM. Worker panics and `runtime.Goexit` produce a completion
failure and a Go-side diagnostic with call locations, omitting the panic value
and request/response data. This internal API is not yet wired to public Tool
invocation or JavaScript Tool publication.

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
Errors never contain authored source, thrown values, stack traces, private
configuration, or response data. Caller cancellation and deadlines remain Go
context errors. The configured timeout returns `apjsruntime.ErrLimitExceeded`,
allowing an executor to report an execution-limit failure separately from a
canceled caller request. If both contexts have expired, the caller's error wins.

## Limits and lifecycle

Every limit must be positive. The caller resolves operator ceilings before
compilation; this package supplies no unlimited defaults.

- `MaxSourceBytes` bounds authored UTF-8 source before parsing. For Tools, the
  script and connector helper library share this allowance.
- `MaxInputBytes` bounds the serialized snapshot and response together.
- `MaxOutputBytes` bounds emitted JSON, including punctuation and escaping.
- `MaxDepth` bounds nested JSON values, with the root at depth zero.
- `MaxNodes` bounds JSON values and object property inspections. Combined input
  and output each receive this allowance; it is not a JavaScript instruction count.
- `MaxStackDepth` bounds JavaScript function-call recursion.
- `Timeout` covers input binding, initialization, execution, Promise settlement,
  and result conversion during `Evaluate`, or only initialization during
  `Validate`. An earlier caller deadline wins.

The JSON input/output limits apply to expression and Tool evaluation; Tool
validation receives no input and returns no result data.

A watcher interrupts busy JavaScript loops on cancellation and is joined before
evaluation or validation returns. It uses only Goja's thread-safe interrupt
method; all other runtime work stays on one goroutine. Fresh VMs prevent a late
interrupt or a modified global from affecting the next call.

These limits bound processing for trusted administrator code. They do not make
the in-process runtime a hard memory-isolated sandbox. Native Goja built-ins
cannot be preempted by an interrupt, and JavaScript can allocate memory before
output conversion. The caller also owns authorization, aggregate invocation
accounting, output-schema validation, and limits on the final rich-result envelope.
