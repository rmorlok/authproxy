# proxy

This package sends HTTP requests through a connection. Its authenticator supplies
credentials; the proxy applies them, sends through the instrumented HTTP factory,
and can retry once after recovering from an upstream 401. The factory supplies
request events, rate limits, and telemetry. Callers handle permission checks before
using the proxy.

Use a configured connection client when several requests belong to the same
operation:

```go
client, err := conn.HTTPClient(iface.HTTPClientOptions{
    Actor: iface.ActingAs(actor),
    Labels: map[string]string{"operation": "sync"},
})
if err != nil {
    return err
}
response, err := client.Do(request)
```

`Do` returns headers and a streaming body. It takes ownership of the request body
even on failure; the caller must close any returned response body. Request
contexts supply cancellation and deadlines. A context carrying a different actor
is rejected. Background work uses `iface.WithoutActor()` explicitly.

## Labels for individual requests

`DoWithLabels` adds user labels to one request, including its redirects and
credential retry. For example, a sync can label its read and write requests:

```go
response, err := client.DoWithLabels(readRequest, map[string]string{
    "operation": "read",
    "workload": "catalog",
})
```

For this call, `operation=read` overrides the client's `operation=sync` and any
connection user label with the same key. Other captured labels remain present.
Rate limits, request events, and telemetry receive the final label map. A later
`client.Do(request)` still uses `operation=sync`; nil or empty per-call labels
also use the original stack.

Malformed keys or values and reserved `apxy/` keys return
`iface.ErrHTTPClientInvalidLabels` before credentials or network I/O. Labels cannot
replace the captured actor, connection, or request type. The outgoing body is
closed and send evidence records that the rejected call did not send.

The proxy captures connection and actor metadata when it builds the configured
client's factory. The core connection wrapper defers that step until first use.
Each request creates a lightweight HTTP client from the factory. Calls without
extra labels share the factory's lazily built middleware stack and pooled
connections. Labeled calls copy and merge their labels into the captured
snapshot, then build a private stack. Neither path rereads live metadata or
mutates shared clients. Concurrent calls remain independent. Keep a supplied
label map stable during the call. Create a new configured client to pick up
later metadata or policy changes.

The client does not impose a label-byte budget. A bounded execution host must
account for request data before calling it. JavaScript `connection.fetch` does
not yet accept `init.labels`; its binding and byte accounting are a separate step.

## Other proxy entry points

`ProxyRequest` buffers the upstream response into the structured proxy contract.
`ProxyRequestStream` returns a stream, and `ProxyRequestRaw` copies that stream
into a response writer. These entry points and configured clients share
credential application and recovery behavior. Bodies that cannot be replayed do
not receive a credential retry. A target send is tracked separately from
credential refresh traffic so consumers can distinguish a local rejection from
a request that may have affected the upstream system.
