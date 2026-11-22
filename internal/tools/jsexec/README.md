# JavaScript Tool preparation

`jsexec` prepares one JavaScript Tool definition, its selected connector helper
library, and its execution policy. `Compile` captures the script and input/output
schemas without executing JavaScript. `Validate` checks initialization in a
fresh bounded VM without calling `execute` or supplying a connection.

This package currently provides compilation and initialization checks. Execution,
final result-envelope assembly, server policy defaults, and public invocation
integration are separate work. The lower-level
[JavaScript adapter](../javascript/README.md) already owns copied Tool inputs,
the sole connection capability, rich results, and explicit `ToolError` values.

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

## Apply execution policy

Every field in `Limits` must be positive; this package supplies no defaults.
`ResolveLimits` lowers selected operator ceilings with the definition's optional
caps. `Compile` performs this resolution and validates the complete policy.

| Authored cap | Effective allowances lowered |
| --- | --- |
| `timeoutMillis` | `HTTP.Timeout` and `Runtime.Timeout` |
| `maxRequests` | `HTTP.MaxRequests` |
| `maxInputBytes` | `Validation.MaxInputBytes`, for caller arguments |
| `maxOutputBytes` | `Validation.MaxOutputBytes` and `Runtime.MaxOutputBytes` |
| `maxResponseBytes` | `HTTP.MaxResponseBytes`, across decoded response reads |
| `maxEncodedResultBytes` | `MaxEncodedResultBytes`, for the complete result |
| `maxStackDepth` | `Runtime.MaxStackDepth` |

For example, with an operator request ceiling of 20, the definition above records
a ceiling of 2. With an operator output ceiling of 2048 bytes, its authored 4096
byte cap leaves that stricter ceiling at 2048. Large positive caps cannot overflow
duration/byte conversions or raise a ceiling; zero and negative caps are invalid.

Argument bytes, copied configuration, combined VM input, and response data have
different allowances. A small `maxInputBytes` must not accidentally restrict the
connection's configuration. `Render` bounds snapshot copying;
`Runtime.MaxInputBytes` bounds the combined native input. Source, shared native
work/allocation, response streams/headers, per-request bytes, and pending workers
retain their independent operator ceilings. These HTTP, data, and envelope limits
are captured for execution; `Validate` only exercises initialization policy.

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
Admission, argument/output validation, connection execution, and complete result
limits still belong to the invocation path. The current JSON Schema compiler and
evaluator also need the [work bound tracked in #948](https://github.com/rmorlok/authproxy/issues/948)
before public invocation is enabled. Schema compilation is synchronous and has
no cancellation hook; callers must check cancellation around it.
