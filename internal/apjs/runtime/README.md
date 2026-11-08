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
`let fetch` would fail compilation because the runtime reserves that global.

The caller supplies additional host bindings, including names for disabled
capabilities. `fetch` is always reserved; an explicit descriptor is allowed but
cannot preserve enumerability. The compiler copies the descriptors and rejects
invalid, duplicate, or escaped names. `execute` belongs to the program and cannot
be a host binding. Initialization makes each host global nonwritable and
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
    return invocation.PrepareInput(runCtx, vm, runtime.AsyncInput{
        Params: json.RawMessage(`{"id":"42"}`),
        Context: json.RawMessage(`{"job":"customer-sync"}`),
    }, limits, nil)
})
```

`PrepareInput` accepts any JSON value for params and an object for extra context.
Nil documents default to `{}`; nonnil empty or malformed documents fail. It adds
`context.connections`, an empty array when no connections are supplied, and
rejects caller JSON containing that reserved property. Params, extra context,
and all connection snapshots share one byte/node allowance. Depth is checked
with each supplied document's root at zero. Aliases refer to the resulting native
objects without another copy or input charge.

No input aliases are installed. Trusted adapters may bind them during
preparation or use the lower-level `Params` and `Context` fields directly with
their own bounded native input. Preparation must honor `runCtx` and keep VM
values inside this evaluation.

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

## Supplied connections and HTTP

`HTTPHost` supplies request conversion, native response objects, and the worker
completion loop. It installs no globals and selects no clients. `PrepareInput`
uses it to bind each supplied connection's `fetch` method to the captured
dispatcher. Creating a host alone gives JavaScript no HTTP access.

For example, compile this program with no additional host globals:

```javascript
async function execute(params, context) {
    const connection = context.connections.find(c => c.labels.region === "us");
    const response = await connection.fetch(connection.cfg.apiHost + "/customers/" + params.id);
    const customer = await response.json();
    return { name: customer.name };
}
```

Given an already-authorized `dispatcher` implementing `ConnectionDispatcher`
and using an [apjs/http session](../http/README.md), prepare its host in `Evaluate`:

```go
result, err := program.Evaluate(ctx, func(
    runCtx context.Context, vm *goja.Runtime, invocation *runtime.AsyncInvocation,
) error {
    host, err := runtime.NewHTTPHost(runCtx, vm, runtime.HTTPOptions{
        MaxRequestBytes: 64 << 10,
        MaxPendingOperations: 8,
    }, limits)
    if err != nil {
        return err
    }
    // Register cleanup before another VM operation can be interrupted.
    invocation.Close = host.Close
    invocation.Start = host.Start
    invocation.Await = host.Await
    return invocation.PrepareInput(runCtx, vm, runtime.AsyncInput{
        Params: json.RawMessage(`{"id":"42"}`),
        Connections: []runtime.ConnectionInput{{
            Snapshot: json.RawMessage(`{
                "id":"cxn_example", "name":"CRM", "namespace":"root.product.user1",
                "cfg":{"apiHost":"https://api.example.test"}, "labels":{"region":"us"}
            }`),
            Dispatcher: dispatcher,
        }},
    }, limits, host)
})
```

The dispatcher validates labels and sends requests through the already-admitted
connection. For Tools, `internal/tools/httphost.Session` implements this interface.
The adapter owns the session: close it on every exit, link its cancellation to
the context passed to `Evaluate` so HTTP expiry or exhaustion interrupts busy
JavaScript, and map its final error and send evidence into the caller's outcome.
`HTTPHost.Close` cancels its requests and body reads without waiting for workers;
it does not close the session or its clients. Register cleanup immediately after
construction, including when later preparation might fail.

Connections preserve caller order; empty or duplicate IDs fail before authored
code or I/O. Each snapshot accepts only `id`, `name`, `namespace`, `cfg`, `labels`,
and `annotations`. Missing name/namespace become empty strings and missing
objects become `{}`. Configuration must be an object; labels and annotations
must contain string values. Explicit null objects are invalid.

`context.connections`, its array membership, and each connection's top-level
properties are read-only. Nested metadata remains mutable: changing
`connection.cfg.apiHost` affects this VM's next authored URL but does not change
stored configuration or the client's captured identity and attribution. The
non-enumerable `fetch` method stays bound when borrowed. Connection objects carry
a permanent symbol marker that the generic encoder rejects, so scripts return
selected metadata such as `{id: connection.id}` instead of a live capability.
No credentials, client lookup, default connection, or automatic helper-library
selection is exposed. Supplying connections does not enable global `fetch`.

One host can bind several dispatchers. Each function permanently captures its
dispatcher; borrowing the function does not retarget it. Bind handles from the
same HTTP session so requests share its deadline, response-byte allowance, and
send evidence. Fetch calls and body reads across every binding also share the
host's `MaxPendingOperations`. A refused read leaves `bodyUsed` false, allowing a
retry after other work finishes. Nothing dispatches before `Start`, which the
async evaluator calls only after initialization and its Promise jobs succeed.
The lower-level `BindConnectionFetch` and ordinary `BindFetch` APIs remain
available to trusted host adapters that own their native binding layout.

Fetch accepts an absolute HTTP(S) URL and optional `method`, `headers`, and `body`.
Headers are a string record; `Host` sets the request's host override. Bodies are
strings or native `ArrayBuffer` values. The combined request fields must fit
`MaxRequestBytes`; unsupported options fail without dispatch. Responses expose
`status`, `ok`, `headers.get(name)`, `bodyUsed`, and single-use async `text()`,
`json()`, and `arrayBuffer()` readers. HTTP status errors resolve normally.

Dispatchers run off the VM goroutine and must capture only detached Go state.
They take request-body ownership and return bounded responses whose bodies honor
request cancellation. On failure they close any partial response and return only
an error. The shared HTTP handles already meet this contract. Workers transfer
only owned Go data; the VM goroutine settles Promises and decodes JSON. Late
responses are closed without touching the VM. Worker panics and `Goexit` reject
safely and log only fixed classifications and Go source locations, never payloads.

### Connection request labels

Connection fetch also accepts `init.labels`, a string record whose own enumerable
entries are copied before a worker starts:

```javascript
const response = await connection.fetch(connection.cfg.apiHost + "/customers", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify(params),
    labels: {operation: "write", workload: "customer-sync"}
});
```

Label key/value bytes share `MaxRequestBytes` with the URL, method, headers, and
body. Omitted, null, or undefined labels add nothing. Values must be primitive
strings; arrays and enumerable symbol keys are rejected. Inherited and
non-enumerable properties are ignored. Later edits to the authored map cannot
change an admitted request, another call, or the connection's metadata snapshot.

`ConnectionDispatcher.ValidateLabels` runs on the VM owner after bounded copying
and before worker admission. It must perform no I/O or map mutation and must
reject malformed and protected labels according to the adapter's policy. AuthProxy
adapters use the existing user-label validator, including protection of `apxy/`
attribution. Validation failures become fixed invalid-input rejections and do not
consume a logical HTTP request. The runtime stays independent of schema packages.

The worker calls `DoWithLabels` with detached data. The adapter applies request
user labels over captured connection/client user labels, using the same session
for every call. Resolved labels reach rate limits, events, and telemetry without
changing actor, connection, or request-type attribution. Every supplied connection
supports this extension; direct `BindFetch` and global `fetch` reject it.

## Optional direct HTTP

`AsyncInput.DirectDispatch` enables global `fetch` only for an explicitly supplied
direct client. It defaults to nil, leaving `fetch` undefined. Tools always leave
it nil, and pure expressions have no HTTP host. Direct calls apply no connection
credentials or connection-authentication recovery. Authored headers remain
request data; a failed connection call never falls back to this client.

For example, a caller can allow a script to read a public status endpoint:

```javascript
async function execute() {
    const response = await fetch("https://status.example.test/summary");
    return await response.json();
}
```

Use `httpf.NewDirectClient` to capture attribution and reuse the existing HTTP
middleware, then bind it to the same `aphttp.Session` as any connection handles:

```go
client, err := httpf.NewDirectClient(factory, httpf.DirectClientOptions{
    Namespace: "root.jobs",
    RequestType: httpf.RequestTypeGlobal,
    Actor: nil, // Explicit background request without an initiating actor.
    Labels: map[string]string{"job": "status-check"},
})
if err != nil {
    return nil, err
}
direct, err := owner.Bind(client)
if err != nil {
    return nil, err
}

// Inside the existing Evaluate preparation callback, after registering the
// same host's Close, Start, and Await callbacks:
input.DirectDispatch = func(request *http.Request) (*aphttp.Response, error) {
    response, _, err := direct.Do(request)
    return response, err
}
return invocation.PrepareInput(runCtx, vm, input, limits, host)
```

Here `httpf` is `internal/httpf`, and `owner` is the caller-owned execution
session. Its aggregate send evidence remains available even when JavaScript
catches a failed request. The adapter captures namespace, request type, actor,
and labels before execution; JavaScript cannot replace them. No default client
is created when the capability is omitted.

Direct and connection calls use one host and one HTTP session, sharing request
count, decoded response bytes, pending workers, and deadline. They have the same
Fetch subset and body-read behavior. A supplied direct dispatcher requires a
host even when there are no connections. Initialization cannot dispatch through
either path; cleanup cancels both, including late responses. The `labels` fetch
option remains unsupported for direct requests.

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
