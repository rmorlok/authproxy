# HTTP Tool execution

This package runs a compiled HTTP Tool through a configured connection client.
It validates arguments, builds the request, reads a bounded response, maps it
into a rich Tool result, and checks successful structured output. Each invocation
has its own timeout, HTTP session, and copied connection context.

For example, a contact lookup can use a tenant from connection configuration and
an ID supplied by the caller:

```yaml
description: Look up a contact
verbs: [read]
inputSchema:
  type: object
  required: [id]
  properties:
    id: {type: string}
outputSchema:
  type: object
  required: [name]
  properties:
    name: {type: string}
proxyHttp:
  method: GET
  url: https://api.example.com/{{path.cfg.tenant}}/contacts/{{path.params.id}}
  response:
    transformJavascript: '({name: response.body.displayName})'
```

With tenant `acme` and arguments `{"id":"contact-42"}`, the executor sends
`GET https://api.example.com/acme/contacts/contact-42`. If the provider returns
`{"displayName":"Ada","privateDebug":"..."}`, the result's `structuredContent`
is `{"name":"Ada"}`. The output schema checks the projected result.

## Compile and invoke

Compile the decoded `tools.ToolDefinition` once for its captured revision and
operator policy. `Limits` groups the existing renderer, schema validator,
response mapper, and HTTP host limits. Required operator limits must be positive;
`Compile` lowers them with the Tool's optional `limits` before compiling
the components. Omitted Tool limits inherit operator policy, and larger Tool
values cannot raise it. Zero or negative Tool values fail compilation.

```go
program, err := httpexec.Compile(definition, limits)
if err != nil {
    return err
}

// connection and actor were resolved and authorized by invocation admission.
client, err := connection.HTTPClient(iface.HTTPClientOptions{
    Actor: iface.ActingAs(actor),
})
if err != nil {
    return err
}
result, err := program.Execute(ctx, client, httpexec.Invocation{
    ID: "tiv_01example0000001",
    Arguments: common.RawJSON(`{"id":"contact-42"}`),
    Snapshot: render.Input{
        ToolID: "tol_01example0000001", 
		Revision: 7,
        ConnectionID: "cxn_01example0000001",
        ConnectionNamespace: "root.product.owner",
        Cfg: map[string]any{
			"tenant": "acme"
		},
    },
})
```

The imports are `internal/tools/httpexec`, `internal/tools/render`,
`internal/core/iface`, and `internal/schema/common` from this module. The IDs are
examples; production calls use the identities captured during admission.
See the [renderer](../render/README.md), [schema validator](../validation/README.md),
[response mapper](../httpcodec/README.md#response-limits), and
[HTTP host](../httphost/README.md) for their limit fields and units.

For example, the contact lookup above can declare:

```yaml
limits:
  timeoutMillis: 1500
  maxInputBytes: 1024
  maxResponseBytes: 65536
  maxOutputBytes: 8192
  maxEncodedResultBytes: 16384
```

With a 60-second operator timeout, this Tool gets 1.5 seconds for the entire
invocation. A stricter one-second operator timeout still wins. Arguments are
limited to 1 KiB, provider response bytes to 64 KiB, and result content to 8 KiB.
The response can be larger than the result because a projection can select just
the fields the caller needs. The complete serialized result, including metadata
and base64 expansion, must also fit within 16 KiB. Each allowance remains subject
to any smaller operator limit.

Tool limits apply to these components:

| Tool field | Execution allowance |
| --- | --- |
| `timeoutMillis` | HTTP session and response-transform time |
| `maxRequests` | Logical HTTP client calls; credential recovery uses existing proxy policy |
| `maxInputBytes` | Raw caller arguments, including JSON whitespace |
| `maxOutputBytes` | Result content, structured-output validation, and transform output |
| `maxResponseBytes` | Decoded response reads and response mapping |
| `maxEncodedResultBytes` | Complete serialized result envelope |
| `maxStackDepth` | Response-transform JavaScript call depth |

The argument cap does not reduce the renderer's snapshot/request allowance or
the transform's combined snapshot/response input allowance. Those contain
connection configuration and provider data as well as arguments. Source size,
nesting, work, numeric precision, encoded response streams, and headers retain
their separate operator limits. Response and output byte caps are independent;
this compiler does not add a shared total across them. No defaults or unlimited
values are introduced here.

The program owns its compiled source and can be reused concurrently. Each call
validates `Arguments` and replaces `Snapshot.Params` with that private decoded
tree. Configuration, labels, annotations, and provenance are then copied for
request templates and response projection. Never include credentials in the
snapshot. Do not modify invocation data while a call is using it. Returned
results own their content and metadata.

## Responses and failures

Status rules run before reading a body. Non-2xx responses and explicit error
rules for 2xx responses close the stream without reading or decompressing it.
Thus an error body's malformed gzip or private text cannot replace the chosen
status error. Successful bodies use the HTTP host's decoding and byte limits;
only its filtered headers reach response transforms.

The executor performs one logical client call. It enforces the smaller of the
host and mapper response-byte caps while reading, before buffering a response
the mapper would reject. It closes streams before mapping and schema checking.
Existing proxy credential recovery can retry a replayable request; this package
does not add retries based on status rules, schema failures, or retry advice.

Successful JSON, text, image, audio, and resource content follow the mapper's
rich-result rules. An output schema validates only successful
`structuredContent`; errors skip it. A configured schema requires present
structured JSON, so a binary-only response fails unless a projection produces
the promised JSON. Missing output schemas leave rich passthrough available.

Failures use fixed messages and discard partial output:

| Failure | Result code |
| --- | --- |
| Invalid arguments or request/context rendering | `INPUT_INVALID` |
| Unavailable connection credentials | `CONNECTION_UNAVAILABLE` |
| Transport or response-read failure | `UPSTREAM_FAILED` |
| Invalid HTTP response or content encoding | `UPSTREAM_PROTOCOL_ERROR` |
| Invocation timeout, request allowance, input validation data limit, or transform runtime limit (including its output encoder) | `EXECUTION_LIMIT_EXCEEDED` |
| Response/output data limits or result-envelope size | `OUTPUT_TOO_LARGE` |
| Invalid successful output or output-schema mismatch | `OUTPUT_INVALID` |

Authored status rules keep their configured code, message, and retry advice.
Every error preserves the host's send evidence: `notStarted` requires proof that
no target request was sent; possible sends remain `mayHaveExecuted`. A successful
HTTP status alone does not establish that the whole Tool action completed.

Caller cancellation/deadline returns its Go context error. Invalid internal
identities/client/program return `ErrInvalidInvocation`. If even a safe failure
envelope cannot fit, execution returns `httpcodec.ErrEncodedResultLimit` with no
partial result. A policy timeout can still produce a result while the caller's
context remains live.

Admission can set `Invocation.Deadline` to its absolute policy deadline so time
spent preparing the connection and compiling the Tool reduces the time available
to execute it. The earlier of that deadline, the compiled timeout, and the caller's
deadline applies. Leaving it zero starts the policy timeout in `Execute`. An
expired policy deadline returns `EXECUTION_LIMIT_EXCEEDED` with `notStarted` if no
request was sent; expiration after a possible send retains `mayHaveExecuted`.

## Integration boundary

Core's private `invokeHTTPTool` helper connects authorization and connection
preparation to this executor using explicit operator limits. It uses
`ResolveLimits` and `ErrorResult` to return bounded readiness failures before
compilation, and supplies one deadline for preparation and execution. Operator
configuration and the public invocation endpoint are still separate work.

The caller must authorize the Tool aliases for the connection, check readiness
and any expected revision, and capture the same revision/configuration for the
program, snapshot, and configured client. This package performs no admission or
proxy ACL checks and registers no endpoint. It does not cache revisions or
resolve policy defaults.

The shared timeout covers the invocation. Snapshot copying is bounded synchronous
work and observes cancellation afterward. Schema validation checks cancellation
only around the library's synchronous evaluation and currently has no evaluation
step budget. The schema-evaluation bound remains required before public Tool
invocation is enabled; no work is detached after a cancellation response.

Run `go test -race ./internal/tools/httpexec ./internal/tools/httpcodec` for the
executor composition and status/body-decision tests.
