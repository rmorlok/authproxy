# OAuth proxy send evidence

These scenarios exercise `_proxy` and the OAuth callback through real HTTP
listeners. The OAuth provider independently records refresh and resource
requests; a unique query marker identifies each target call without relying on
response status or comparing clocks between the host and container.

| Scenario | Target requests | Evidence | Wrapped behavior |
| --- | ---: | --- | --- |
| Expired token; refresh rejected | 0 | `notSent` | Ordinary AuthProxy error |
| Expired token; refresh succeeds; target quota exhausted | 0 | `notSent` | Outer 200 with synthetic upstream 429 |
| Target 401; refresh; successful replay | 2 | `sent` | Replayed upstream 200 |
| Target 401; refresh; replay connection dropped | 2 | `sent` | Original upstream 401 body and headers retained |
| Target 401; refresh; replay quota exhausted | 1 | `sent` | Outer 200 with synthetic upstream 429 |

The quota cases install a real per-connection token bucket scoped to `proxy`
requests. Its single token refills over an hour, so neither OAuth traffic nor a
fixed-window boundary can make the test pass unexpectedly. The cache snapshot is
installed synchronously before the call.

Target calls use POST with a JSON body. AuthProxy can rebuild this body for its
401 recovery, while `net/http` will not silently retry it when the provider drops
the connection before response headers. Provider wildcard resource scripts are
kept sequential and cleaned up after each scenario.

Run this coverage against both database providers from `integration_tests`:

```sh
AUTH_PROXY_TEST_DATABASE_PROVIDER=sqlite go test -tags integration ./oauth2 -run '^TestProxySendEvidence_' -count=1
AUTH_PROXY_TEST_DATABASE_PROVIDER=postgres go test -tags integration ./oauth2 -run '^TestProxySendEvidence_' -count=1
```

See [the proxying guide](../../docs/src/content/docs/sdks/proxying.md) for the
public meaning of each evidence value and retry guidance.
