# Bounded HTTP execution

This package sends prepared requests through supplied clients with one timeout,
logical request allowance, and aggregate decoded response allowance per execution. It owns
request accounting, response decoding, body cleanup, and target-send evidence.
It has no JavaScript, connection lookup, authorization, or Tool result logic.

For example, an action may read an account and then update it. Keep one session
alive for both requests so they consume the same allowances. The following
helper performs the read through an already configured `Client`:

```go
func readAccount(ctx context.Context, client aphttp.Client) ([]byte, error) {
    session, err := aphttp.New(ctx, aphttp.Limits{
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

    bound, err := session.Bind(client)
    if err != nil {
        return nil, err
    }
    request, err := http.NewRequestWithContext(
        session.Context(), http.MethodGet,
        "https://api.example.test/account", nil,
    )
    if err != nil {
        return nil, err
    }
    response, _, err := bound.Do(request)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        return nil, errors.New("account lookup failed")
    }
    body, err := io.ReadAll(response.Body)
    if err != nil {
        return nil, err // Discard partial data after a read failure.
    }
    return body, nil
}
```

Here `aphttp` is `github.com/rmorlok/authproxy/internal/apjs/http`; `http` is
`net/http`. Limits are examples, not defaults. The caller resolves effective
policy and admits the client before constructing a session. The
[Tool adapter](../../tools/httphost/README.md) supplies configured AuthProxy
clients without importing their authentication or instrumentation into this
package. The session owns the budget; each `ClientHandle` returned by `Bind`
permanently captures one client and that owner. There is no default client or
client lookup from request data.

Callers reporting partial effects can capture the second return value from
`ClientHandle.Do`: it describes that call's target-send evidence, including on error.
The session also retains aggregate evidence across calls.

## Binding several clients

Create the owner once, then call `session.Bind(crmClient)` and
`session.Bind(billingClient)` with clients already admitted by the caller.
Send requests through each returned handle's `Do` method. Both clients keep their
own authentication and attribution, while their calls use the same session
context, deadline, allowances, and aggregate evidence.

For a session with `MaxRequests: 2`:

| Call | Remaining requests |
| --- | --- |
| CRM client rejects a call before target I/O. | 1: the logical handoff still counts. |
| Billing client returns a response. | 0: both clients spend the same allowance. |
| A newly bound handle attempts another request. | Rejected with `ErrRequestLimit`; no client call occurs. |

Binding performs no I/O and grants no fresh allowance. Binding an ended or
exhausted owner is permitted, but its handles reject dispatch under that same
policy. A handle cannot be retargeted or moved to another owner; copying it or
borrowing its `Do` method keeps the original binding. Handles have no independent
close operation. Close the owner to cancel requests and reads across all clients.
A session with no handles is valid and makes no requests.

## Client and ownership contracts

`Client.Do` returns a raw `*http.Response`, a `SendState`, and an error. The
adapter must collect evidence across the complete logical call, including
redirects and credential recovery. It keeps credential endpoint traffic separate
from target traffic. The session neither creates clients nor adds retries;
authentication, destination/Host handling, attribution, and instrumentation stay
with the supplied client. Clients may be reused across sessions and must support
concurrent calls. Closing a session never closes or reconfigures the client.

`ClientHandle.Do` owns the request body on entry, including local rejection. After
handoff, the client owns the body and any replay readers supplied by `GetBody`.
Every returned raw response body transfers from the client to the session,
including responses returned alongside an error. The session either closes it
or returns it as a bounded `Response.Body`, which the caller must close.

Responses expose status, captured content type, a detached header map, and a lazy
body. They omit `http.Response.Request`, which can contain credentials. Headers
omit known authentication, cookie, and hop-by-hop fields, including fields named
by `Connection`. All raw headers are validated and charged before filtering.
The filter cannot identify secrets in arbitrary provider headers or bodies;
domain adapters choose which data to return.

Use `Response.ContentType` to interpret the body. It retains the original media
type even if the header was filtered or the caller edits the copied header map.
Status and header access never read content, including gzip headers. An unwanted
response can be closed without draining or decoding it.

## Allowances and cancellation

All limit fields must be positive:

| Limit | Scope |
| --- | --- |
| `Timeout` | Whole session lifetime, including preparation and gaps between requests. |
| `MaxRequests` | Logical client handoffs, including calls later proven unsent. |
| `MaxResponseBytes` | Decoded bytes accepted across all reads, including partial or discarded responses. |
| `MaxStreamBytes` | Each response before this package decodes it; not a wire-byte measurement. |
| `MaxHeaderBytes` | Each raw response header map, including names, values, and line overhead. |

Local validation or admission failures consume no logical request. Handoffs and
accepted response bytes are never refunded. Exceeding the aggregate response
allowance cancels the session, including pending requests and reads. Each body
uses bounded scratch space and charges chunks before returning bytes. Concurrent
reads may consume an extra chunk before detecting exhaustion; excess bytes never
reach the caller. These limits are not a process memory quota.

Body reads support identity and gzip (including `x-gzip`). Already decompressed
transport responses are not decoded twice. When the host decodes gzip, it removes
`Content-Encoding` and `Content-Length`. Bodyless responses such as HEAD, 204,
and 304 retain representation metadata and return EOF without decoding.
Unsupported or invalid encodings fail only when content is read.

`NewWithDeadline` can include earlier preparation time using an absolute policy
deadline; it cannot extend the configured timeout or caller deadline. Context
values come from the session parent. Each request context can shorten its
deadline or cancel it without replacing those values. Caller cancellation keeps
its context error; policy expiry returns `ErrTimeout`. `Session.Err()` supplies
the same distinction for work between requests.

`Close` cancels I/O and releases timers without waiting for client calls or body
cleanup. Closing a response releases that request without canceling siblings.
Late responses are still closed or transferred under the same ownership rules.
Cancellation cannot undo upstream effects.

## Errors and send evidence

Errors have fixed categories and contain no raw URL, body, credential, or
transport diagnostics. A trusted adapter can return or wrap
`ErrCredentialsUnavailable` for known unusable credentials; the host strips the
private cause. It never infers that category from error text or status codes.
Other client errors become `ErrTransport`, unless caller or session cancellation
takes precedence. HTTP error statuses remain normal responses.

`ClientHandle.Do` returns evidence for that call even on failure. `Session.SendState()`
reports evidence across all handles and calls:

| State | Meaning |
| --- | --- |
| `SendStateNotSent` | Every target attempt was stopped before transport; also the state before any calls. |
| `SendStateMayHaveSent` | A target send is possible, a call is pending, or evidence is missing. This is the zero value. |
| `SendStateSent` | At least one target transport returned a response, regardless of status or later failure. |

Confirmed sends dominate possible sends, which dominate known-unsent calls.
Invalid client evidence remains uncertain. A local response such as a rate-limit
429 does not prove a target send; only the adapter's transport evidence can do
that. A recovered client panic remains uncertain because its call did not return
complete evidence.

Evidence is provisional while calls are pending. If all later return known-unsent,
the aggregate can become known-unsent. Closing the session does not wait for
that outcome. A caller that already reported uncertainty must keep that reported
result; late evidence cannot revise it. Neither a confirmed send nor an HTTP
success proves that the caller's complete logical action finished.
