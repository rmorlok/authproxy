# Connection-bound HTTP execution

A Tool can make several requests to complete one action, such as finding a CRM
contact and then updating it. `httphost.Session` sends those requests through one
captured connection's configured HTTP client. The requests share a timeout, a
logical request allowance, and an aggregate decoded response allowance.

The existing connection proxy still supplies credentials, credential recovery,
rate limiting, and request instrumentation. The host receives prepared
`http.Request` values; [httpcodec](../httpcodec/README.md) can produce them from
Tool templates and map their responses to rich results.

## Send requests through the captured connection

The following helper reads an account response. `conn` is the connection already
selected and authorized by the invocation caller. The limits below are examples;
the caller must resolve effective policy before creating a session. `ctx` carries
the actor admitted for this invocation.

```go
func readAccount(ctx context.Context, conn iface.Connection) ([]byte, error) {
    client, err := conn.HTTPClient(iface.HTTPClientOptions{
        Actor:  iface.ActingAs(authcore.ActorFromContext(ctx)),
        Labels: map[string]string{"operation": "read-account"},
    }) // An empty RequestType defaults to proxy traffic.
    if err != nil {
        return nil, err
    }
    session, err := httphost.New(ctx, client, httphost.Limits{
        Timeout:          30 * time.Second,
        MaxRequests:      3,
        MaxResponseBytes: 2 * 1024 * 1024,
        MaxStreamBytes:   2 * 1024 * 1024,
        MaxHeaderBytes:   32 * 1024,
    })
    if err != nil {
        return nil, err
    }
    defer session.Close()

    req, err := http.NewRequestWithContext(
        session.Context(), http.MethodGet,
        "https://api.example.test/account", nil,
    )
    if err != nil {
        return nil, err
    }
    response, err := session.Do(req)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        // A status rule can reject a response without reading its body.
        return nil, errors.New("account lookup failed")
    }
    body, err := io.ReadAll(response.Body)
    if err != nil {
        return nil, err // Never use a partial body as a complete response.
    }
    return body, nil
}
```

The module imports are `internal/apauth/core` (aliased as `authcore`),
`internal/core/iface`, and `internal/tools/httphost` under
`github.com/rmorlok/authproxy`; the other imports are from the Go standard library.
For a multi-request action, keep one session alive around the entire action.
Creating a session for each page would reset the allowances and timeout.

The client captures actor attribution, request type, and labels and uses the
connection's existing streaming proxy for credentials and recovery. Every client
requires an explicit actor choice. Construction fails if `ActingAs` has no valid
actor; background work with no initiating actor can choose `iface.WithoutActor()`.
A session context carrying an actor must match the client's configured actor.
Create a client per invocation so its captured labels and rate-limit configuration
are current. `Session` only requires its `Do` method. The invocation caller must
supply a client configured from the admitted connection; the host does not select
a connection or configure attribution itself.

`Do` takes ownership of the outgoing body immediately, even when a canceled
context or exhausted budget prevents a proxy call. After handoff, the existing
proxy owns that body, including any fresh replay readers supplied by `GetBody`.
Caller-provided headers, URL, and host overrides retain existing proxy behavior.

The returned response contains only status, captured content type, copied headers,
and a bounded body. It does not expose the proxy's credential-bearing
`http.Response.Request`.
Known authentication and cookie headers, including `Set-Cookie` and
`Authentication-Info`, are removed. Hop-by-hop headers and every field nominated
by `Connection` are also removed. Other end-to-end metadata such as `Retry-After`
and rate-limit headers remains available. All raw headers count toward the header
limit and are validated before filtering; decoding instructions are captured
before their headers can be removed.
This filter does not identify secrets in arbitrary provider-specific headers or
bodies. Trusted Tool definitions still control what data reaches the caller.

`Response.ContentType` preserves the original media type and its parameters,
even if `Connection` names `Content-Type` or the caller changes `Response.Header`.
Pass this field to `httpcodec.ResponseInput.ContentType` when mapping the body;
reading the content type from the filtered header map could misclassify JSON as
binary data.

`Do` does not read the response body, including a gzip header. Callers can inspect
status and close an unwanted response without draining it. A caller that needs
the body must read it and handle errors before transforming the result.

## Allowances and cancellation

All limit fields must be positive:

| Limit | What it counts |
| --- | --- |
| `Timeout` | The entire session lifetime, including rendering and time between requests. |
| `MaxRequests` | Logical client calls, including calls that fail after handoff. |
| `MaxResponseBytes` | Decoded bytes read across all responses, including partial bodies later discarded. |
| `MaxStreamBytes` | Bytes from each proxy response body before decoding performed by this host. |
| `MaxHeaderBytes` | Each copied response header map's names, values, and field-line overhead. |

Credential recovery, redirects, and credential refresh can cause extra HTTP
attempts within a logical client call. The host adds no retries. It never refunds
consumed requests or response bytes after a failure.

Response reads support identity and gzip encoding, including the legacy `x-gzip`
alias defined in [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110.html#section-8.4.1.3).
Go's HTTP transport sometimes decompresses gzip itself; the host recognizes that
case and does not decode twice. When the host decodes gzip, its returned headers
omit `Content-Encoding` and `Content-Length`. Bodyless responses such as `HEAD`, `204`, and `304` retain
their representation metadata and return EOF without decoding. Other encodings
fail when a response body is read. The stream cap
is not a wire-byte measurement because the transport may already have decoded it.

Concurrent calls share synchronized accounting. Each body lazily allocates a
32 KiB scratch buffer when reading content and reuses it for subsequent reads,
charging each decoded chunk before returning it. Terminal reads release the
buffer; inspecting status without reading does not allocate it. Reads already
in flight can consume an extra chunk each when another read exhausts the budget;
those excess bytes are never returned. An exact-limit body can require a
one-byte probe to distinguish EOF from excess. These limits bound host processing
and exposed data; they are not a hard memory quota for the transport or process.

Exceeding the aggregate decoded allowance cancels the whole session. The read
that detects excess fails with `ErrResponseLimit`, and cancellation interrupts
other pending requests and reads with that same error. Future requests are
rejected too. Catching the first error does not restore the invocation's budget.
Fully returned responses remain usable, but partial bodies
must not be treated as complete responses. Cancellation does not undo upstream
side effects.

The session context preserves the admitted actor and other context values;
the configured client fixes request attribution and rejects a conflicting actor.
Each request's context can cancel that request sooner but cannot replace the
session's identity. Caller cancellation returns `context.Canceled` or
`context.DeadlineExceeded`; the configured session timeout returns `ErrTimeout`.
`Session.Err()` makes the same distinction for work between requests.
Closing a session cancels outstanding I/O and releases its timer. Closing a
response releases its request without canceling other calls in the session.

## Errors and execution uncertainty

Proxy and body failures return fixed error classifications, without URLs,
credential text, response bodies, or upstream error messages. Callers can use
`errors.Is` and map them to Tool error results. Non-context errors returned by
the proxy currently use `ErrTransport`, including credential-resolution failures.
The proxy does not yet expose stable categories that distinguish revoked
credentials from database, decryption, or configuration failures. Those categories
must be added at their source before an executor can reliably recommend
reconnecting. Local rate-limit rejections are 429 responses, not Go errors, and
remain available to status rules.

`ExecutionState()` starts as `notStarted`. Each client call is uncertain while
in flight. Once it returns, the session uses the proxy's send evidence:

| What happened | State after the call |
| --- | --- |
| Credentials could not be resolved before any target attempt. | `notStarted` |
| A local rate limiter returned 429 before any target attempt. | `notStarted` |
| The target transport was entered, even if DNS, TLS, or the request later failed. | `mayHaveExecuted` |
| The client or transport did not supply reliable evidence. | `mayHaveExecuted` |

These examples assume no earlier call may have executed. A possible send stays
recorded for the rest of the session. For example, a target 401 followed by a
local 429 during the credential retry remains `mayHaveExecuted`. Redirects also
retain earlier attempts. OAuth token requests do not count as target requests.
Every client handoff still consumes a logical request, including a proven
pre-send rejection; execution state does not refund the allowance.

This evidence records entry into the target transport, not proof that the
provider received or completed a request. A successful HTTP response also does
not establish completion of the whole logical action. A higher-level executor
decides when the Tool action is complete. Unknown clients remain conservative,
including core client initialization failures that do not yet report evidence.

This package does not load Tool revisions, check permissions, validate input or
output schemas, render requests, or build Tool results. The invocation layer must
authorize the captured Tool and connection before creating a session. Existing
public proxy permission checks are unchanged.
