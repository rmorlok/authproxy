# Tool HTTP adapter

This package connects a Tool's configured AuthProxy client to the shared
[HTTP session](../../apjs/http/README.md). The shared package owns deadlines,
request and response budgets, header filtering, lazy body decoding, and cleanup.
This adapter supplies two AuthProxy-specific translations:

- Configured-client send observations and trusted credential failures become
  neutral HTTP evidence and error categories.
- Aggregate send evidence becomes a Tool's `notStarted` or `mayHaveExecuted`
  execution state.

The existing connection client still owns credentials, recovery, rate limiting,
request events, and telemetry. This package does not select connections, check
permissions, render requests, validate schemas, or build Tool results.

## Use an admitted connection

For example, read an account through a connection already selected and authorized
by the caller. The policy values below are examples; the caller resolves operator
ceilings and authored caps before creating the session.

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
        return nil, errors.New("account lookup failed")
    }
    body, err := io.ReadAll(response.Body)
    if err != nil {
        return nil, err // Never use a partial body as a complete response.
    }
    return body, nil
}
```

The AuthProxy imports are `internal/apauth/core` (aliased as `authcore`),
`internal/core/iface`, and `internal/tools/httphost`. Other imports are from Go's
standard library. `Limits`, `Response`, and the error sentinels refer to the
shared HTTP contracts.

The client's explicit actor choice must match any actor in the session context.
Background work without an initiating actor can use `iface.WithoutActor()`.
Create a client per invocation to capture current labels and rate-limit policy.
Keep one session around a whole multi-request action so pagination or a second
endpoint does not reset its allowances. `NewWithDeadline` can include admission
and preparation time in that same policy deadline.

`Do` owns the outgoing body even on local rejection. The caller owns the returned
body and must close it, including when discarding a status without reading it.
The shared session never closes or reconfigures the supplied client. A response
contains only status, captured content type, filtered copied headers, and a
bounded body; it does not expose a credential-bearing `http.Response.Request`.
Pass `Response.ContentType` to response mapping because filtering can remove the
original header. See the shared package for decoding and accounting details.

## Errors and send evidence

The adapter calls the configured client once, collecting its existing `httpf`
observation across redirects and credential-recovery attempts. OAuth token
requests stay separate from target evidence. A trusted
`auth_methods.ErrCredentialsUnavailable` marker becomes
`ErrCredentialsUnavailable`; other client errors become `ErrTransport`. Neither
error retains private diagnostics. Caller cancellation and policy failures keep
the shared session's precedence.

For example, a missing API-key row is unavailable, while failure to decrypt an
existing key is a transport/operational failure. Error text or a 401/403 status
alone does not establish missing credentials. Failed credential recovery still
returns the original upstream 401 response, and local rate limiting still
returns a normal 429 response. See the
[authentication error contract](../../auth_methods/README.md#error-contract).

The embedded session retains `notSent`, `mayHaveSent`, and `sent` evidence.
`ExecutionState()` maps only `notSent` to the Tool state `notStarted`. Both a
possible send and a confirmed target response map to `mayHaveExecuted`: neither
proves that a multi-request Tool action finished successfully.

For example, a local rate-limit rejection before any target attempt is
`notStarted`. A target 401 followed by a local rejection during recovery is
`mayHaveExecuted`. Every logical client handoff still consumes request budget,
including a proven pre-send rejection.

Outstanding calls remain uncertain. Once all return with proof they were unsent,
the aggregate can return to `notStarted`; earlier possible or confirmed sends
remain recorded. Closing the session does not wait for outstanding clients. A
result returned while work is pending must keep its conservative state rather
than revise it from late evidence. Unknown clients and recovered client panics
also remain conservative.
