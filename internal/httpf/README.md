# httpf

This package implements the HTTP client manager for AuthProxy. AuthProxy uses [Gentleman](https://github.com/h2non/gentleman)
for HTTP clients, and this package automatically applies middleware based on the configuration for session proxy.

## Target-request send evidence

The proxy uses `DoWithSendState` for streaming requests and
`DoGentlemanWithSendState` for structured requests. Both return one of three
states:

| State | Evidence |
| --- | --- |
| `notSent` | The complete factory middleware stack returned without reaching the target transport. |
| `mayHaveSent` | The target transport was entered without a confirmed response, or tracking was incomplete. |
| `sent` | The underlying target transport returned a valid HTTP response, regardless of status. |

A rate limiter can return a local 429 with `notSent`; a provider's 429 produces
`sent`. A target 401 followed by a local 429 during credential recovery retains
`sent`. Redirects likewise retain evidence from earlier target responses.

The factory observes entry into its middleware stack and places a send marker
immediately before the underlying transport. Entering that transport counts as
a possible send, including DNS or TLS failures. Only a valid response observed
below middleware establishes `sent`; synthetic responses cannot establish it.
Body reading, response parsing, or errors returned by later middleware do not
erase an earlier target response. An unknown wrapper cannot establish
`notSent`, even when it delegates to a known stack, but confirmed target responses remain
useful evidence. Middleware must preserve the request context and delegate
outbound work through its supplied transport.

`ObserveSends` creates an observation for one logical client call without
changing `HTTPClient.Do`. The proxy reports each attempt with
`ReportSendState`. Reports accumulate with `sent` taking precedence over
`mayHaveSent`, which takes precedence over `notSent`. No reports and invalid
state values mean `mayHaveSent`. Callers read `State` after the logical client
call returns and treat outstanding calls as uncertain. The send helpers only
return evidence; they do not report to an observation. Callers should default
an attempt to `mayHaveSent` and defer reporting so a panic cannot leave an
earlier local rejection as the final result. If a panic prevents the helper
from returning, that attempt can remain `mayHaveSent` even if a target response
was observed inside it. Any previously reported `sent` still takes precedence.

The original boolean APIs remain compatible: `MayHaveSent` and
`DoWithSendEvidence` are false only for `notSent`. `ReportSendEvidence(true)`
reports `mayHaveSent`, since a boolean does not prove a target response.

The transport tracker is separate from that logical observation and is added
only to the target request after credentials have been resolved. OAuth token
requests therefore do not count as target execution. Gentleman installs its
tracker immediately before dial, after request plugins configure the effective
client and context. Early plugin interception remains uncertain. Tracking
neither changes HTTP status/error behavior nor proves that the provider
completed an action. A missing response may still conceal a completed action.
