# httpf

This packeage implements the HTTP client manager for Auth Proxy. Auth proxy uses [gentlemen](https://github.com/h2non/gentleman)
for http clients, and this package automatically applies middleware based on the configuration for session proxy.

## Target-request send evidence

The streaming proxy uses `DoWithSendEvidence` to distinguish a local rejection
from a possible target request. For example, a rate limiter can return 429
without contacting the provider. By contrast, a target 401 followed by a local
429 during a credential retry must retain the first target attempt.

The factory observes entry into its middleware stack and places a send marker
immediately before the underlying transport. Entering that transport counts as
a possible send, including DNS or TLS failures. A stack that returns locally
without reaching the marker provides evidence of no target send. A transport
without this instrumentation remains uncertain. Middleware must preserve the
request context and delegate outbound work through its supplied transport.

`ObserveSends` creates an observation for one logical client call without
changing `HTTPClient.Do`. The proxy reports each attempt with
`ReportSendEvidence`; a possible send cannot be erased by a later local
rejection. No reports means unknown, not unsent. Callers read `MayHaveSent` after
the client returns and treat an outstanding call as uncertain.

The transport tracker is separate from that logical observation and is added
only to the target request after credentials have been resolved. OAuth token
requests therefore do not count as target execution. Tracking neither changes
HTTP status/error behavior nor proves that the provider completed an action.
