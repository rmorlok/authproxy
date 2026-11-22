# Tool result envelopes

This package builds the result shared by Tool executors: captured Tool,
invocation, and connection identities; structured JSON and rich content; and
errors with trusted execution state. The HTTP response mapper and executor use
it for identity capture, failure replacement, and the final serialized size
check.

## Build a result

For example, a customer lookup can attach its validated JSON to captured
identities and check the complete result against a 16 KiB allowance:

```go
result, err := toolresult.New(
    "tol_01example0000001", 7, "tiv_01example0000001",
    api.ToolResultMetadata{ConnectionID: "cxn_01example0000001"},
    16 * 1024,
)
if err != nil {
    return nil, err
}
result.StructuredContent = common.RawJSON(`{"id":"customer-42","name":"Ada"}`)
return toolresult.Finish(ctx, result, executionState, 16 * 1024)
```

Here `toolresult` imports `internal/tools/result`; `api` and `common` are the
packages under `internal/schema`. `executionState` comes from the executor's
record of outbound requests. It is retained if the result needs to become a
size-limit error.

`New` validates identities and copies optional ToolSet/catalog revision pointers.
It returns an empty, independently owned result. Populate it only with bounded,
validated output owned by this invocation. Explicit JSON `null` stays present;
nil structured content stays absent. `Finish` measures the whole JSON envelope,
including identities, metadata, escaping, rich content, and error details.

## Replace output with a failure

When an executor detects a failure, it supplies a safe error and the execution
state established by its HTTP evidence:

```go
toolresult.SetFailure(result, &api.ToolResultError{
    Code: "OUTPUT_INVALID",
    Message: "The Tool result does not match the output schema.",
}, executionState)
return toolresult.Finish(ctx, result, executionState, 16 * 1024)
```

`SetFailure` discards structured output, rich content, provider metadata, and
the previous error. It takes ownership of the supplied error and sets its
execution state. Callers validate authored errors and choose retry advice;
this package does not infer whether retrying is safe.

If the completed envelope is too large, `Finish` replaces its payload with a
small `OUTPUT_TOO_LARGE` error. That replacement also removes previous error
details and retry advice, while keeping the captured identities and execution
state. If the replacement cannot fit, it returns `ErrEncodedResultLimit` and
no result. Caller cancellation takes precedence over either outcome.

## Ownership and execution limits

`SetFailure` and `Finish` modify their supplied result. Each invocation owns its
result and must not share it with concurrent writers. These helpers do not copy
or revalidate arbitrary output graphs. Executors remain responsible for output
schemas, content validation, and limits on data before passing it here.

The byte check uses synchronous JSON serialization, with cancellation checks
around that work. It bounds the returned envelope, not the memory allocated by
the serializer. The caller keeps its execution deadline alive through this
step and handles policy expiry separately when forming a timeout result.
