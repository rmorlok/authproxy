# JavaScript Tool execution

`jsexec` prepares one JavaScript Tool definition, its selected connector helper
library, and its execution policy. `Compile` captures the script and input/output
schemas without executing JavaScript. `Validate` checks initialization without
calling `execute` or supplying a connection. `Execute` runs the prepared Tool in
a fresh VM against one admitted connection and assembles its bounded result.

The lower-level [JavaScript adapter](../javascript/README.md) owns copied Tool
inputs, the sole connection capability, rich results, and explicit `ToolError`
values. This executor adds argument/output schemas, invocation identities, and
final envelope limits. Core's private invocation path supplies finite server
policy from `tools.invocation`, authorizes the captured revision, and selects its
pinned connector helpers and configured connection client. Public invocation
integration remains separate work.

## Prepare a customer lookup

A connector can provide a reusable path helper:

```javascript
function customerPath(id) {
    return "/customers/" + encodeURIComponent(id);
}
```

The Tool can use that helper inside its entry point:

```javascript
async function execute(params, context) {
    const [connection] = context.connections;
    const url = new URL(customerPath(params.id), connection.cfg.apiBase).href;
    const response = await connection.fetch(url);
    if (!response.ok) {
        throw new ToolError("CUSTOMER_LOOKUP_FAILED", "Could not read customer", {
            upstreamStatus: response.status
        });
    }
    return await response.json();
}
```

Given that source as `script`, the helper source as `helperSource`, and a complete
operator policy as `limits`, prepare the definition as follows:

```go
library, err := apjs.CompileAndValidateLibraryWithContext(ctx, helperSource, limits.Runtime)
if err != nil {
    return err
}
definition := toolschema.ToolDefinition{
    Javascript: util.ToPtr(script),
    InputSchema: common.RawJSON(`{
        "type": "object",
        "properties": {"id": {"type": "string"}},
        "required": ["id"],
        "additionalProperties": false
    }`),
    Limits: &toolschema.ExecutionLimits{
        MaxRequests: util.ToPtr(2),
        MaxOutputBytes: util.ToPtr(int64(4096)),
    },
}
program, err := jsexec.Compile(definition, library, limits)
if err != nil {
    return err
}
if err := program.Validate(ctx); err != nil {
    return err
}
```

The caller chooses the library from the captured connector revision. Passing nil
means no helper library. Library source must be admitted before its own parsing;
the context-aware compiler above preserves it for combined source accounting.
`jsexec.Compile` then checks Tool and library source against one
`Runtime.MaxSourceBytes` allowance and applies the Tool host-name restrictions.

Compilation accepts exactly the `javascript` executor. It parses the required
object input schema and optional successful-output schema, retaining their
distinction between omission and a boolean schema such as `false`. It copies the
effective policy and library wrapper, so later edits to definition buffers,
authored limit pointers, or the caller's library value cannot change the program.
Reuse it only for the same captured revision, selected library, and policy.

## Execute an admitted lookup

Once admission has authorized the Tool and prepared its configured HTTP client,
call the compiled program with the matching captured context:

```go
result, err := program.Execute(ctx, client, tools.Invocation{
    ID: "tiv_01example0000001",
    Arguments: common.RawJSON(`{"id":"customer-42"}`),
    Snapshot: render.Input{
        ToolID: "tol_01example0000001", Revision: 7,
        ConnectionID: "cxn_01example0000001",
        ConnectionNamespace: "root.product.owner",
        Cfg: map[string]any{"apiBase":"https://crm.example.test"},
    },
})
```

`tools.Invocation` is the shared input from `internal/tools`. `client` is an
`iface.HTTPClient` configured for that connection and actor.
Admission must ensure that the program, snapshot, and client belong together.
The caller supplies raw argument JSON; `Execute` validates it and replaces any
`Snapshot.Params` with the validated values. Connection configuration comes
from the snapshot; credentials stay in the client. Arguments must contain finite
JavaScript numbers. Invalid caller arguments produce `INPUT_INVALID`; unusable
captured configuration produces `CONNECTION_UNAVAILABLE`. Snapshot byte, depth,
and step limits produce `EXECUTION_LIMIT_EXCEEDED`.

The customer example receives `params.id == "customer-42"` and one entry in
`context.connections`. It can call multiple endpoints through that connection;
all requests share the HTTP allowances and captured attribution. Global direct
`fetch` is absent. Request label overrides retain the configured client's
middleware selection and protected-label checks.

Successful structured output is checked against the optional output schema.
Rich results may omit structured content when no schema is configured. Explicit
`ToolError` results skip the success schema, retain their public details, and
receive execution state from HTTP evidence. They do not acquire automatic retry
advice. The shared [result helpers](../result/README.md) then bound the complete
envelope, including provider metadata and error details.

Uncaught ordinary exceptions produce a fixed `OUTPUT_INVALID` failure. When the
rejected result is an actual fetch or body-read error, its safe host category
determines the public code:

| Uncaught host failure | Result code |
| --- | --- |
| Unavailable credentials | `CONNECTION_UNAVAILABLE` |
| Request, worker, or request-byte limit; timeout | `EXECUTION_LIMIT_EXCEEDED` |
| Response, stream, or header limit | `OUTPUT_TOO_LARGE` |
| Invalid response or unsupported/invalid content encoding | `UPSTREAM_PROTOCOL_ERROR` |
| Transport or response-read failure | `UPSTREAM_FAILED` |

The runtime recognizes the original error object by private identity, so copying
its properties or writing a matching message cannot supply a host classification.
Exception text, transport diagnostics, and credentials never enter these errors.
A script can catch a per-request failure and return a useful result or throw an
explicit `ToolError`. Rethrowing the original error preserves its classification;
throwing a different ordinary error produces `OUTPUT_INVALID`, even after a caught
credentials failure. An earlier handled failure does not override the script's
chosen outcome.

Exhausting the aggregate response allowance remains terminal and produces
`OUTPUT_TOO_LARGE`; catching its rejection cannot reset the allowance. Native
work/output encoding limits and policy timeouts produce `EXECUTION_LIMIT_EXCEEDED`.
Output-schema mismatches produce `OUTPUT_INVALID`; output validation data limits
and complete-envelope limits produce `OUTPUT_TOO_LARGE`.

Failure results retain a single snapshot of send evidence taken when evaluation
ends: `notStarted` requires no possible target send, and possible sends remain
`mayHaveExecuted`. An HTTP response alone cannot prove completion of a multi-step
Tool action. Caller cancellation returns a Go context error. Invalid internal
inputs return `ErrInvalidInvocation`; if even a safe failure cannot fit, execution
returns `result.ErrEncodedResultLimit` with no result.

## Apply execution policy

Every field in `Limits` must be positive; this package supplies no defaults.
`ResolveLimits` lowers selected operator ceilings with the definition's optional
caps. `Compile` performs this resolution and validates the complete policy.
The server's defaults and native-budget formula are documented in the
[configuration reference](../../../docs/src/content/docs/reference/configuration.md#tool-invocation-limits).

| Authored cap | Effective allowances lowered |
| --- | --- |
| `timeoutMillis` | `HTTP.Timeout` and `Runtime.Timeout` |
| `maxRequests` | `HTTP.MaxRequests` |
| `maxInputBytes` | `Validation.MaxInputBytes`, for caller arguments |
| `maxOutputBytes` | `Validation.MaxOutputBytes` for successful structured output; `Runtime.MaxOutputBytes` for ordinary results, complete `toolResult` / `ToolError` payloads, and each console record |
| `maxResponseBytes` | `HTTP.MaxResponseBytes`, across decoded response reads |
| `maxEncodedResultBytes` | `MaxEncodedResultBytes`, for the complete result |
| `maxStackDepth` | `Runtime.MaxStackDepth` |

For example, with an operator request ceiling of 20, the definition above records
a ceiling of 2. With an operator output ceiling of 2048 bytes, its authored 4096
byte cap leaves that stricter ceiling at 2048. Large positive caps cannot overflow
duration/byte conversions or raise a ceiling; zero and negative caps are invalid.

For JavaScript, `maxOutputBytes` also bounds an explicit `ToolError`'s code,
message, and details together, including JSON field names and escaping. When
logging is enabled, each console record's message and optional fields share this
byte cap. Exceeding it while encoding a result or error fails conversion; an
oversized console record ends execution with a limit failure.

For example, `maxOutputBytes: 32` allows `{saved: true}`, but cannot fit the
`CUSTOMER_LOOKUP_FAILED` error shown above. A long `console.log(...)` message
can also exhaust that cap even when the eventual result would fit. Choose the
cap to accommodate all three uses. `maxEncodedResultBytes` separately bounds
the final result envelope, including error details and trusted identities;
raising it does not increase the earlier runtime encoding allowance.

Argument bytes, copied configuration, combined VM input, and response data have
different allowances. A small `maxInputBytes` must not accidentally restrict the
connection's configuration. `Render` bounds snapshot copying;
`Runtime.MaxInputBytes` bounds the combined native input. Source, shared native
work/allocation, response streams/headers, per-request bytes, and pending workers
retain their independent operator ceilings. These HTTP, data, and envelope limits
are captured for execution; `Validate` only exercises initialization policy.

`Execute` uses the earlier runtime/HTTP timeout for the whole invocation.
`tools.Invocation.Deadline` can include time already spent in admission and preparation;
an earlier caller deadline also applies. The enclosing context stays live after
the JavaScript adapter closes its dedicated HTTP session, so schema checks and
envelope assembly remain under the original deadline. Policy expiry can still
produce a safe error when the original caller context remains live.

Known argument and structured-output JSON passes consume the execution's shared
native work/allocation ledger, along with managed VM and HTTP conversion work.
Snapshot copying, schema data, and final envelope serialization also have their
own finite limits. These do not supply a schema evaluation-step budget or a hard
memory quota; synchronous bounded operations check cancellation on completion.

## Check initialization

`Validate(ctx)` initializes the selected library and Tool under the effective
runtime time, stack, and work/allocation limits. Shared pure utilities,
`toolResult`, and `ToolError` are available. Arguments, connection configuration,
HTTP, and authored logging are absent. The example above validates because its
connection access occurs inside `execute`, which this check never calls.

A top-level `context.connections[0].fetch(...)` fails. A top-level constant or
helper declaration is allowed. Uncaught initialization exceptions and unhandled
Promise rejections fail with safe runtime categories; caller cancellation stays
a context error and policy exhaustion is `apjsruntime.ErrLimitExceeded`.
Each check uses a fresh VM, including concurrent checks of one program.

Initialization checks do not prove that every invocation will return valid output.
`Execute` performs argument/output validation, connection execution, and complete
result limits; admission remains caller-owned. The current JSON Schema compiler and
evaluator also need the [work bound tracked in #948](https://github.com/rmorlok/authproxy/issues/948)
before public invocation is enabled. Schema compilation is synchronous and has
no cancellation hook; callers must check cancellation around it.
