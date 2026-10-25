# Connection-bound HTTP execution

A Tool can make several requests to complete one action, such as finding a CRM
contact and then updating it. `httphost.Session` sends those requests through one
captured connection. The requests share a timeout, a logical request allowance,
and an aggregate decoded response allowance.

The existing connection proxy still supplies credentials, credential recovery,
rate limiting, and request instrumentation. The host receives prepared
`http.Request` values; [httpcodec](../httpcodec/README.md) can produce them from
Tool templates and map their responses to rich results.

## Send requests through the captured connection

The following helper reads an account response. `conn` is the connection already
selected and authorized by the invocation caller. The limits below are examples;
the caller must resolve effective policy before creating a session.

```go
func readAccount(ctx context.Context, conn iface.Connection) ([]byte, error) {
    session, err := httphost.New(ctx, conn, httphost.Limits{
        Timeout:          30 * time.Second,
        MaxRequests:      3,
        MaxResponseBytes: 2 * 1024 * 1024,
        MaxStreamBytes:   2 * 1024 * 1024,
        MaxHeaderBytes:   32 * 1024,
    }, map[string]string{"operation": "read-account"})
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

The module imports are `internal/core/iface` and `internal/tools/httphost` under
`github.com/rmorlok/authproxy`; the other imports are from the Go standard library.
For a multi-request action, keep one session alive around the entire action.
Creating a session for each page would reset the allowances and timeout.

`Do` takes ownership of the outgoing body immediately, even when a canceled
context or exhausted budget prevents a proxy call. After handoff, the existing
proxy owns that body, including any fresh replay readers supplied by `GetBody`.
Caller-provided headers, URL, and host overrides retain existing proxy behavior.

The returned response contains only status, copied headers, and a bounded body.
It does not expose the proxy's credential-bearing `http.Response.Request`.
`Do` does not read the response body, including a gzip header. Callers can inspect
status and close an unwanted response without draining it. A caller that needs
the body must read it and handle errors before transforming the result.

## Allowances and cancellation

All limit fields must be positive:

| Limit | What it counts |
| --- | --- |
| `Timeout` | The entire session lifetime, including rendering and time between requests. |
| `MaxRequests` | Logical calls to the connection proxy, including calls that fail after handoff. |
| `MaxResponseBytes` | Decoded bytes read across all responses, including partial bodies later discarded. |
| `MaxStreamBytes` | Bytes from each proxy response body before decoding performed by this host. |
| `MaxHeaderBytes` | Each copied response header map's names, values, and field-line overhead. |

Credential recovery, redirects, and credential refresh can cause extra HTTP
attempts within a logical proxy call. The host adds no retries. It never refunds
consumed requests or response bytes after a failure.

Response reads support identity and gzip encoding. Go's HTTP transport sometimes
decompresses gzip itself; the host recognizes that case and does not decode
twice. When the host decodes gzip, its returned headers omit `Content-Encoding`
and `Content-Length`. Bodyless responses such as `HEAD`, `204`, and `304` retain
their representation metadata and return EOF without decoding. Other encodings
fail when a response body is read. The stream cap
is not a wire-byte measurement because the transport may already have decoded it.

Concurrent calls share synchronized accounting. A read uses at most 32 KiB of
scratch space and charges its decoded chunk before returning it. Reads already
in flight can consume an extra chunk each when another read exhausts the budget;
those excess bytes are never returned. An exact-limit body can require a
one-byte probe to distinguish EOF from excess. These limits bound host processing
and exposed data; they are not a hard memory quota for the transport or process.
Exceeding the aggregate decoded allowance cancels the whole session.

The session context supplies the admitted actor and other context values.
Each request's context can cancel that request sooner but cannot replace the
session's identity. Caller cancellation returns `context.Canceled` or
`context.DeadlineExceeded`; the configured session timeout returns `ErrTimeout`.
`Session.Err()` makes the same distinction for work between requests.
Closing a session cancels outstanding I/O and releases its timer. Closing a
response releases its request without canceling other calls in the session.

## Errors and execution uncertainty

Transport and body failures return fixed error classifications, without URLs,
credential text, response bodies, or upstream error messages. Callers can use
`errors.Is` and map them to Tool error results.

`ExecutionState()` is `notStarted` until the first proxy handoff. After that it
remains `mayHaveExecuted`, including when a later call is rejected or a transport
error occurs. The proxy cannot always determine whether the upstream received a
failed request, and a successful HTTP response does not establish completion of
the whole logical action. A higher-level executor decides when the Tool action
is complete.

This package does not load Tool revisions, check permissions, validate input or
output schemas, render requests, or build Tool results. The invocation layer must
authorize the captured Tool and connection before creating a session. Existing
public proxy permission checks are unchanged.
