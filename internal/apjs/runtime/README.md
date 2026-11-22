# Bounded JavaScript runtime primitives

This package compiles and evaluates pure expressions and async JavaScript
programs in fresh Goja VMs. It owns the
execution budget, deadline, stack limit, interrupt watcher, JSON conversion,
and Promise rejection tracking. Adapters supply native input and optional host capabilities;
the runtime initializes the program, calls it, and waits for its result.

`CompileExpression` prepares one strict expression without running it. The
compiled expression can serve concurrent calls, each with its own VM. For
example, copy `{"price":6,"quantity":7}` into `params` and receive `{"total":42}`:

```go
limits := runtime.Limits{
    MaxSourceBytes: 64 << 10,
    MaxInputBytes: 64 << 10,
    MaxOutputBytes: 64 << 10,
    MaxWorkUnits: 64 << 20,
    MaxAllocationUnits: 64 << 20,
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

The preparation callback is trusted integration code. It binds copied JSON on the calling goroutine, using the same limits passed
to compilation. The runtime has already installed its pure utilities. It must honor `runCtx`, return safe errors, and retain no VM
values. Input preparation, expression execution, and result conversion all use
the same execution budget and deadline. A preparation error prevents the expression from running.
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
capabilities. `fetch` and the shipped pure utility names are always reserved;
an explicit descriptor is allowed but cannot preserve enumerability. The compiler copies the descriptors and rejects
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
Use the same limits in that enclosing call so it installs the pure utilities
and owns the execution budget before `Initialize` runs.

`PromiseTracker` counts unhandled rejections without inspecting private reasons.
Install one before authored initialization and defer `Close`. `CheckRejections`
rejects any failure still unhandled after a VM call drains its microtasks.
`Validate` and `Evaluate` manage tracking themselves; validation never calls the
entry point, and evaluation owns root settlement and result conversion.

During the initial `execute` call, the tracker temporarily records rejected
Promise identities because the root can reject and acquire a handler before the
call returns. Once the root is known, it releases that map and retains only the
root's identity and unhandled status alongside the aggregate count. Detached
rejections during later host work are counted without retaining their promises
or reasons. This lets extensions interpret the exact root while unrelated
unhandled rejections still fail execution.

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
encoding share one execution budget and deadline. Callers receive only detached JSON; failure or
cancellation discards the result. Hosts return safe errors unchanged, while
authored exceptions and rejection values remain private.

## Authored structured logging

Async execution can receive an explicit logging capability. The adapter supplies
its existing `*slog.Logger` through `AsyncInvocation.Logger`; a nil logger leaves
`console` unavailable. There is no fallback to the process default logger.
Pure expressions and validation receive no authored logger.

For example, an adapter can capture the job's identity in its configured logger
before running the program:

```go
jobLogger := logger.With("job_id", "sync-example")
result, err := program.Evaluate(ctx, func(
    runCtx context.Context, vm *goja.Runtime, invocation *runtime.AsyncInvocation,
) error {
    invocation.Logger = jobLogger
    return invocation.PrepareInput(runCtx, vm, runtime.AsyncInput{
        Params: json.RawMessage(`{"count":12}`),
    }, limits, nil)
})
```

The program can then report a selected event:

```javascript
async function execute(params, context) {
    console.info("Page processed", {count: params.count, complete: true});
    return {processed: params.count};
}
```

The record's top-level message is always `"JavaScript authored log"`. Authored
content is nested beneath `javascript`, separate from the host's attribution:

```json
{
  "msg": "JavaScript authored log",
  "job_id": "sync-example",
  "javascript": {
    "message": "Page processed",
    "fields": {"count": 12, "complete": true}
  }
}
```

`console.debug`, `info`, `log`, `warn`, and `error` take a primitive string message
and an optional plain JSON object of fields. `log` uses the info level. Omitting
fields, or passing `undefined`, omits the `fields` attribute. Nested objects,
dense arrays, strings, finite numbers, booleans, and null are supported inside
the field object. A top-level array or null, extra arguments, accessors, proxies,
cycles, and other unsupported values raise a fixed, catchable `TypeError`.
Methods return `undefined`; there is no format-string interpolation or arbitrary
argument printing. String decoding replaces lone UTF-16 surrogates with U+FFFD.

The runtime captures conversion intrinsics before authored code runs. It does
not call coercion hooks, getters, or `toJSON`, and passes only detached Go data
to the logger. `console` and its methods are protected; borrowing a method keeps
its original logger. Authored keys such as `trace_id`, `connection_id`, or `level`
stay inside `javascript.fields` and cannot replace host attribution. Keep trusted
attributes outside the reserved `javascript` group. Passing `runCtx` to `LogAttrs`
preserves the existing logging/telemetry pipeline's trace
context; the runtime creates no telemetry provider or configuration toggle.

Logging becomes available only after initialization and its Promise checks
succeed. It closes before host cleanup and result interpretation. Earlier
records remain emitted if later execution fails. The runtime never logs input,
configuration, credentials, HTTP bodies, results, or exception text automatically;
only the values explicitly passed to these methods become authored log content.

Each record shares `MaxOutputBytes` and `MaxNodes` across its message and fields;
each value starts at depth zero under `MaxDepth`. Conversion and emission consume
the execution's cumulative work/allocation budget, including temporary JSON and
the detached Go tree. Disabling a level in the logger suppresses emission but
still validates, converts, and charges the call. Budget exhaustion is terminal;
catching an error cannot permit another log or HTTP request. Native JSON decoding
is bounded by the admitted record and checks cancellation before and after it.
The supplied logging handler is trusted synchronous code: it must honor context
and return promptly, since the runtime cannot interrupt a blocked handler.

The HTTP host's `HTTPOptions.Logger` remains its separate internal diagnostic
sink. Supplying that option does not enable `console`, and Tool adapters do not
yet expose this authored logging capability.

## Trusted result extensions

Use `CompileExtendedAsync` when an adapter needs a typed Go result instead of
JSON. It takes the same source, library, host bindings, and limits as
`CompileAsync`, plus a `ResultExtension[T]`. Ordinary `AsyncProgram.Evaluate`
continues to return JSON.

For example, a report adapter could let JavaScript return a branded text result:

```javascript
async function execute(params, context) {
    return report("Processed " + params.count + " records");
}
```

The adapter reserves `report` and installs a pure helper in each fresh VM. Its
private map distinguishes helper results from ordinary objects:

```go
type Report struct { Text string }

extension := runtime.ResultExtension[Report]{
    Bindings: []runtime.HostBinding{{Name: "report"}},
    Install: func(_ context.Context, vm *goja.Runtime) (runtime.ResultInterpreter[Report], error) {
        payloads := make(map[*goja.Object]goja.Value)
        if err := vm.Set(
			"report", 
			func(call goja.FunctionCall) goja.Value {
				marker := vm.NewObject()
                payloads[marker] = call.Argument(0)
                return marker
            },
		); err != nil {
            return runtime.ResultInterpreter[Report]{}, runtime.ErrInvalidInput
        }
        return runtime.ResultInterpreter[Report]{
            Fulfilled: func(
				_ context.Context, 
				value goja.Value, 
				encoder *runtime.ResultEncoder,
			) (Report, error) {
                marker, ok := value.(*goja.Object)
                if !ok { return Report{}, runtime.ErrInvalidOutput }
            
                payload, branded := payloads[marker]
                if !branded { return Report{}, runtime.ErrInvalidOutput }
            
                text, err := encoder.String(payload)
                if err != nil { return Report{}, err }
            
                return Report{Text: text}, nil
            },
        }, nil
    },
}
program, err := runtime.CompileExtendedAsync(source, nil, nil, limits, extension)
```

Call `program.Validate(ctx)` to check initialization, then `program.Evaluate`
with the same preparation callback used for ordinary async programs. Validation
installs the pure helpers but supplies no inputs or HTTP hosts and never invokes
`execute` or the result interpreter. Initialization can use these helpers in
both paths. Their names cannot be redeclared by source or a helper library and
are frozen before authored initialization.

`Fulfilled` is required and defines both branded and ordinary success behavior.
The example rejects ordinary values; another adapter can encode them with
`encoder.Encode(value)`. An optional `Rejected` callback can recognize a branded
root failure and return a domain result or safe error. If it does not recognize
the value, the runtime returns `ErrEvaluation`. Ordinary thrown values must stay
private. An unrelated unhandled Promise rejection prevents interpretation;
rejections created during interpretation also discard the result.

The runtime closes outstanding hosts before interpretation. It captures the
encoder's intrinsics before installation and keeps the owner goroutine and
deadline active throughout conversion. `Encode` returns detached JSON; `String`
accepts only primitive strings and rejects lone UTF-16 surrogates. Raw JSON
preserves those units as escapes. Every field converted by either method shares
one byte/node allowance, with depth measured separately from each field's root.
Text is charged as JSON, including quotes and escaping. A failed conversion
remains an error even if the callback ignores it; cancellation also discards the
entire result.

Extension callbacks are trusted Go code. They must honor their context, use the
encoder for authored payloads, and return no VM values, callbacks, or host
objects inside `T`. Installation performs no I/O and creates fresh per-VM state;
it must support concurrent evaluations. Domain validation and any final envelope
size limit remain the adapter's responsibility. These helpers do not create a
hard memory sandbox or change HTTP send evidence.

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
strings, native `ArrayBuffer` values, or genuine `URLSearchParams` instances. Form bodies are serialized and receive
`Content-Type: application/x-www-form-urlencoded;charset=UTF-8` unless the caller
supplied a content type. The serialized body and generated header count toward
the same request allowance. The combined request fields must fit
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

## Pure utilities and the execution budget

Every bounded VM includes `atob`, `btoa`, `TextEncoder`, `TextDecoder`, `URL`, and
`URLSearchParams`. They are available during validation, initialization,
expressions, async execution, and trusted result conversion. Their global names
are reserved, nonwritable, nonconfigurable, and non-enumerable. They perform no
I/O; direct fetch, supplied connections, and authored logging remain separate
capabilities.

`Run` owns the execution's remaining `MaxWorkUnits` and `MaxAllocationUnits`.
It carries that budget in `runCtx`, which adapters pass through all managed
operations. Initial input conversion, utilities, result encoding, and managed
HTTP request/response work consume the same remaining allowances. A nested
`Run` also respects its own limits and the enclosing execution's remaining
budget. Entering another helper or package never grants a fresh parent allowance.
Independent top-level evaluations, including validation, have separate budgets.
HTTP calls refused before dispatch and repeated response metadata access also
consume allowance, even when they send no request.

An adapter that performs managed conversion before or after `Run` can call
`WithExecutionBudget(ctx, limits)` once and pass the returned context through
that entire operation. `Run` then inherits that parent allowance. Trusted
adapters charge additional managed work through `ChargeWork` and
`ReserveAllocation`; both use the context's existing ledger. That ledger holds
only Go accounting state, never a VM or Goja callback, so HTTP worker contexts
cannot retain a runtime through it.

The counters describe managed native work and storage:

- A work unit counts a native entry, an admitted byte or UTF-16 code unit to
  process, or a visited value/property. Scanning the same data again in another
  operation consumes work again. These are accounting units, not JavaScript
  instructions or elapsed CPU time.
- An allocation unit counts a buffer byte, a string UTF-16 code unit, or a
  native value/tuple slot. Temporary copies, returned strings, and retained
  state all consume allowance. The units bound storage by fixed multiples;
  they do not measure the process heap. Charges are cumulative and are not
  refunded when a temporary is discarded or a query parameter is deleted.

Fixed VM bootstrap, intrinsic capture, and utility binding setup are bounded by
the implementation and covered by the execution deadline. The cumulative
counters meter managed operations and data; they do not bill every internal
setup allocation or instruction.

The caller sets both fields alongside the ordinary limits:

```go
limits.MaxWorkUnits = 64 << 20
limits.MaxAllocationUnits = 64 << 20
```

Input and final-output byte limits retain their payload meanings. For example,
a predicate can allow 1 MiB of input but return at most five JSON bytes for a
Boolean. Intermediate work still shares its execution budget even when the
result is tiny. Depth, stack, node, HTTP request, and response caps continue to
bound their respective structures and operations.

Budget exhaustion permanently fails the execution with `ErrLimitExceeded`,
even if authored code catches an error or a trusted adapter ignores one. No
later HTTP request can start; earlier sends retain their evidence. Native work
also observes the shared deadline and caller cancellation. Arbitrary Goja
allocations and instructions are not fully accounted for. In particular,
some public reflection APIs allocate key lists before exposing their size.
This remains a bounded host runtime for trusted code, not a hard heap sandbox.

## Base64 helpers

`atob` and `btoa` operate on **binary strings**, where each character represents
one byte. For example, `btoa("\u00ff")` returns `"/w=="`, and
`atob("/w==").charCodeAt(0)` returns `255`. They do not encode Unicode as UTF-8;
`btoa("雪")` fails because that character does not fit in a byte.

```javascript
const prefix = btoa("item:");
async function execute(params, context) {
    return { encoded: btoa(atob(prefix) + params.id) };
}
```

With `params.id` equal to `"42"`, this returns `{"encoded":"aXRlbTo0Mg=="}`.
`atob` uses
[forgiving base64 decoding](https://infra.spec.whatwg.org/#forgiving-base64-decode):
it accepts the standard alphabet, optional valid padding, and ASCII tab, LF, FF,
CR, and space. Unused trailing bits are ignored, so both `"YQ"` and `"YR"`
decode to `"a"`. URL-safe `-` and `_` are rejected.

Both functions accept only primitive strings. Objects, boxed strings, and other
values are rejected without invoking coercion or getters. Malformed input raises
a catchable `TypeError` with fixed text. This differs from browser argument
coercion and `DOMException` errors; this package does not install a DOM layer.

Calls and admitted UTF-16 input units consume shared work, including malformed
attempts and whitespace. The exact result length consumes allocation units
before allocation: ASCII bytes for `btoa`, binary-string code units for `atob`.
Malformed input produces no result allocation charge.

## Text codecs

`TextEncoder` and `TextDecoder` provide bounded UTF-8 conversion. The
constructors also work during initialization and no-I/O validation:

```javascript
const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8", {fatal: true});

async function execute(params, context) {
    const bytes = encoder.encode(params.text);
    return { utf8: Array.from(bytes), text: decoder.decode(bytes) };
}
```

For `params.text = "café"`, the result is
`{"utf8":[99,97,102,195,169],"text":"café"}`. Encoding returns a native
`Uint8Array`; use `Array.from(bytes)` when returning JSON. An already-supplied
connection can send `body: bytes.buffer`, and a decoder can consume
`await response.arrayBuffer()`. The codecs grant no HTTP access.

The supported [Encoding Standard](https://encoding.spec.whatwg.org/#api) subset is:

- `new TextEncoder().encode(text)` accepts a primitive string and returns UTF-8
  bytes. Omitted or undefined input means empty text. Lone UTF-16 surrogates
  become U+FFFD; valid pairs encode as one scalar. `encodeInto` is not installed.
- `new TextDecoder(label, {fatal, ignoreBOM})` supports UTF-8 only. The label
  defaults to `"utf-8"`; accepted aliases are `utf8`, `unicode-1-1-utf-8`,
  `unicode11utf8`, `unicode20utf8`, and `x-unicode20utf8`, with ASCII case folding
  and surrounding ASCII whitespace ignored. Other encodings raise a fixed
  `RangeError`.
- `decoder.decode(input)` accepts a native `ArrayBuffer` or any native typed
  array/DataView, reading only that view's byte range. Omitted or undefined input
  means empty bytes; null, detached buffers, proxies, and lookalikes are rejected.
  Malformed UTF-8 becomes replacement characters, grouping incomplete valid
  prefixes: bytes `E2 82` produce one U+FFFD. `fatal: true` instead raises a fixed
  `TypeError` without reserving partial output.
- By default, an initial UTF-8 BOM is stripped. `ignoreBOM: true` preserves it.
  Each decode call starts fresh; `{stream: false}` is accepted, but
  `{stream: true}` is rejected. Streaming and legacy encodings are not implemented.

Constructors require `new`. Their globals and prototype methods are read-only;
instances expose read-only `encoding` (`"utf-8"`), plus `fatal` and `ignoreBOM`
on decoders. Methods require genuine codec receivers from this VM. Inputs are
not coerced: boxed strings and non-string encoder inputs fail. Options must be
ordinary or null-prototype objects containing own data Boolean fields; missing,
undefined, or null options use defaults. Accessors on recognized fields and
proxy options fail without running their code. Inherited and unknown fields are
ignored, and option values are not converted to Booleans. These restrictions
differ from browser argument conversion. Error text never contains labels or
input data.

Codec construction and native method entries consume shared work, including
failed argument checks. Each constructed instance also reserves one native
metadata slot. Their data charges use the execution's same counters:

| Operation | Additional work units | Allocation units |
| --- | --- | --- |
| Decoder label | Supplied UTF-16 code units, including whitespace | None |
| `encode` | Input UTF-16 code units | UTF-8 bytes |
| `decode` | Bytes in the supplied buffer/view | UTF-16 code units |

Boolean options have constant size and no separate data charge. Exact output
size is reserved before allocation. These charges accumulate with input copying,
other helpers, and final result encoding; a codec has no separate allowance.

## URLs

`URL` parses an absolute HTTP(S) URL or resolves a relative path against an
HTTP(S) base. For example, `..` removes one path segment from the base:

```javascript
const endpoint = new URL("../contacts?status=open", "https://api.example/v1/users/");
endpoint.href;     // "https://api.example/v1/contacts?status=open"
endpoint.origin;   // "https://api.example"
endpoint.pathname; // "/v1/contacts"
endpoint.search;   // "?status=open"
```

The parsed state is immutable. Instances remain extensible JavaScript objects;
adding or shadowing properties does not change that state. The supported surface is:

| Operation | Behavior |
| --- | --- |
| `new URL(input, base?)` | Accepts primitive strings. An omitted or undefined base requires an absolute input; a supplied base must itself be an absolute HTTP(S) URL, even when the input is absolute. |
| `href`, `origin`, `protocol` | Read-only serialized URL, origin, and scheme with its trailing `:`. |
| `username`, `password`, `host`, `hostname`, `port` | Read-only authority components. `host` includes a nondefault port; `hostname` does not. |
| `pathname`, `search`, `hash` | Read-only path, query, and fragment. A nonempty query or fragment includes its leading `?` or `#`. |
| `toString()` / `toJSON()` | Explicitly returns the same string as `href`. |

Construction requires `new`. Non-HTTP(S) schemes, invalid URLs, boxed strings,
and other values are rejected without object coercion. Lone UTF-16 surrogates
in strings become U+FFFD. Methods and getters require a genuine instance from
the current VM; fake or proxy receivers fail without invoking their hooks.
Invalid inputs and receivers raise a fixed, catchable `TypeError`; parser
diagnostics and input URLs are not included. Installed global and prototype
properties are protected against reassignment and redefinition.

There are no setters, `URL.parse`, `URL.canParse`, object URL APIs, or
`url.searchParams`. Use `new URLSearchParams(endpoint.search)` for an independent
query; editing it does not change the URL. Fetch still accepts a primitive URL
string, so pass `endpoint.href` explicitly:

```javascript
async function execute(params, context) {
    const connection = context.connections[0];
    const endpoint = new URL("contacts", connection.cfg.apiBase);
    const response = await connection.fetch(endpoint.href);
    return response.json();
}
```

With the connection's `cfg.apiBase` set to `"https://api.example/v1/"`, this requests
`https://api.example/v1/contacts` through the supplied connection. URL parsing
itself grants no HTTP capability. The safe result converter rejects URL objects
and does not call their `toJSON` methods; return `endpoint.href` or explicitly
call `endpoint.toJSON()` when a result needs the URL string.

Parsing uses [WHATWG URL parser v0.6.2](https://github.com/nlnwa/whatwg-url/tree/v0.6.2),
the version compatible with this repository's Go 1.25 requirement, with the
HTTP(S) restriction above. Its query API is not exposed; the bounded
`URLSearchParams` implementation owns that surface. Before each native parse,
the host reserves `max(1, n)²` from both shared work and allocation budgets.
A supplied base is parsed separately with `n` equal to its normalized UTF-8
byte length. Resolution uses the input's normalized UTF-8 byte length plus the
serialized base length. This conservative allowance covers the parser's
repeated prefix copies and serialization; it is not a measurement of heap use.
Cached component strings and subsequent getter/serializer results also consume
the shared budget.

String normalization polls cancellation. The native parser has no cancellation
hook: admission bounds its work, and the host checks cancellation immediately
before and after each parse. Budget exhaustion is terminal; catching a
JavaScript exception cannot restore the allowance or permit a later request.

## Query strings

`URLSearchParams` parses and edits query strings. Build a query from an existing
filter and an unencoded search term:

```javascript
const query = new URLSearchParams("status=open");
query.append("q", "café + news");
query.append("status", "new");
query.toString();
// "status=open&q=caf%C3%A9+%2B+news&status=new"
```

The result has no leading `?`. An already-supplied connection can use it with
`connection.fetch("https://api.example/items?" + query.toString())`.

You can also pass an instance directly as a form POST body:

```javascript
async function execute(params, context) {
    const form = new URLSearchParams();
    form.append("grant_type", "client_credentials");
    form.append("scope", "read write");
    return (await context.connections[0].fetch("https://api.example/token", {
        method: "POST",
        body: form,
    })).json();
}
```

Fetch sends `grant_type=client_credentials&scope=read+write` with
`Content-Type: application/x-www-form-urlencoded;charset=UTF-8`. An explicit
content-type header, including an empty one, is preserved. Fetch uses the
protected native serializer, so it ignores an instance's overridden `toString`
and rejects fake or proxy receivers. Serialization and managed HTTP preparation
consume the same execution budget; the body and inferred header also count
against the per-request byte cap. Later mutations do not change the request or
its replay body. This works with connection fetch and an explicitly supplied
direct fetch. If you pass `body: form.toString()` instead, set the content type
yourself; plain string bodies do not infer it.

The supported [URL Standard](https://url.spec.whatwg.org/#interface-urlsearchparams)
subset uses ordered name/value pairs, including duplicates:

| Operation | Behavior |
| --- | --- |
| `new URLSearchParams(text)` | Parses a primitive string; omitted or undefined means empty. Removes one initial `?`, skips empty `&` segments, and splits each pair at its first `=`. |
| `append(name, value)` | Adds another pair at the end. |
| `set(name, value)` | Replaces the first matching pair and removes later duplicates; appends if absent. |
| `get(name)` / `getAll(name)` | Returns the first value or null / a new native array of all values. |
| `has(name, value?)` | Tests for the name, optionally requiring a matching value. |
| `delete(name, value?)` | Removes all matching pairs. An omitted or undefined value matches any value. |
| `sort()` | Stably sorts names by UTF-16 code units, preserving the order of equal names. |
| `size` | Read-only number of pairs, including duplicates. |
| `toString()` | Serializes using form encoding, preserving the current order. |

Parsing changes `+` to a space and decodes valid `%HH` bytes. Invalid escapes
stay literal. Malformed UTF-8 uses replacement characters; a BOM remains data.
Serialization changes spaces to `+` and leaves only ASCII letters, digits,
`*`, `-`, `.`, and `_` unescaped. Other UTF-8 bytes become uppercase percent
escapes. For example, `new URLSearchParams("q=a+b%2Bc").get("q")` returns
`"a b+c"`. Passing a full URL treats it as query text; it does not extract the
URL's query component.

Methods accept primitive strings, replacing lone UTF-16 surrogates with U+FFFD.
Mutation arguments are unencoded data: `append("q", "%2B")` serializes as
`q=%252B`. There is no implicit object or scalar coercion. Object/sequence/copy
constructors, iterators, `forEach`, and coupling to a `URL` object are not
implemented. Use `new URLSearchParams(existing.toString())` to copy a query.
Constructors require `new`; methods require genuine instances from the current
VM. Invalid arguments or receivers raise a fixed, catchable `TypeError` without
running their getters, proxy traps, or conversion hooks. Installed global and
prototype properties are protected against reassignment and redefinition.

Query accounting uses the same execution budget as other managed operations:

- Construction, methods, and `size` reads consume work, including failed native
  argument checks. Each constructed instance reserves one metadata slot. Supplied UTF-16 code units also consume work. Operations that
  scan stored state charge its name/value units plus one unit per pair, even
  when they find a match early. `append` and `size` do not scan. Sort additionally
  charges each moved tuple and both names in each comparison.
- Temporary UTF-8 bytes and normalized UTF-16 strings produced during parsing
  or argument conversion consume allocation units, plus one unit per retained
  pair. Reads charge returned string units; `getAll` also charges one unit per
  array element. Serialization charges its exact ASCII length. Sort reserves
  one scratch slot per pair when there are at least two pairs.

Native parsing, comparison, sorting, and serialization check cancellation.
Deleting a parameter does not refund either execution allowance.

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

Each `Decode` call has its own byte/depth/node boundary checks. HTTP response
readers use these for each body after the HTTP host has bounded its bytes. They
set `MaxBytes` to the accepted body's length, keeping HTTP payload size separate
from the initial `Limits.MaxInputBytes`. All calls using `runCtx` also consume
the same cumulative execution work/allocation budget; a new decode does not
reset it. Standalone decoder calls outside `Run` retain only their explicit
conversion boundary limits. During execution, adapters must pass `runCtx`.

## Limits and errors

All limit fields must be positive, including `MaxWorkUnits` and
`MaxAllocationUnits`. `CompileExpression` and `CompileAsync` enforce
source bytes; adapters that compile other program types enforce their own cap. All
input adapters submit their complete batch with the effective JSON limits. The
decoder enforces those byte/depth/node caps. The runtime enforces timeout, stack
depth, and the final output's byte/depth/node limits. Input and output have
separate structural allowances, while all managed conversion work and allocation
share the execution budget. A later domain envelope needs its own byte cap.
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

### Output-rejection reasons

`OutputReason(err)` lets trusted Go callers distinguish rejected output without
changing the error shown to API clients. For example, an expression returning
`({get count() { return 12; }})` fails with `OutputAccessor`; the getter is never
called:

```go
result, err := program.Evaluate(ctx, nil)
if reason, ok := runtime.OutputReason(err); ok {
    logger.WarnContext(ctx, "JavaScript output rejected", "output_reason", reason)
}
```

The error text remains `JavaScript result is not a supported JSON value`, and
`errors.Is(err, runtime.ErrInvalidOutput)` still succeeds. The extractor also
works through ordinary error wrappers. Expressions, async results, and trusted
`ResultEncoder` conversions use the same reasons. A result encoder keeps its
first failure even if an extension ignores it or attempts another conversion.

| Reason | Meaning |
| --- | --- |
| `unsupported-value` | A non-JSON primitive, such as undefined, a symbol, BigInt, or a non-finite number. |
| `unsupported-object` | An unsupported object kind, such as a function, Proxy, or reflected Go value. |
| `custom-prototype` | An object or array has an unsupported prototype. |
| `accessor` | A property uses a getter or setter instead of a data value. |
| `cycle` | An object repeats on the current traversal path. |
| `symbol-key` | An object has a symbol property key. |
| `array-shape` | An array is sparse or has unsupported extra properties. |
| `invalid-string` | Typed UTF-8 text contains an unpaired UTF-16 surrogate; raw JSON still preserves its escape. |
| `reflection-failure` | Native property metadata could not be read safely. |
| `unexpected-type` | A trusted adapter requires another root type, such as an object, boolean, or string. |
| `invalid-shape` | A trusted adapter rejects the structure of otherwise supported data. |

Adapters can return `RejectOutput(reason)` for their own output contract checks.
It accepts only these fixed reasons; an unknown reason is discarded and produces
the ordinary `ErrInvalidOutput` without diagnostic metadata.

Reasons contain no values, source, property names, array indices, or retained VM
state. Paths are intentionally omitted: keys can contain private data, and
redacted paths would not reliably identify the field. Limit failures have no
output-rejection reason. If cancellation, timeout, or shared-budget exhaustion
overrides a rejection, the final error has no stale reason. These diagnostics
do not enable automatic logging or change the accepted output values.

## Package boundary

This package uses Goja, the pinned URL parser, and the neutral HTTP primitives
under `internal/apjs/http`. It has no imports from root `apjs`, `tools`, core,
configured HTTP-client integration, or schema packages. Domain adapters
choose input names, capabilities, authorization, and result interpretation.
Bounded root `apjs` connector adapters use the same execution budget and pure
utilities. Compatibility entry points that do not use this bounded lifecycle
remain separate APIs.
