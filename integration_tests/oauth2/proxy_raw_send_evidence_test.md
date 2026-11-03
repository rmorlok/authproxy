# Raw OAuth proxy send evidence

These scenarios reuse the wrapped OAuth rig's real API and public listeners,
independent provider request logs, and per-connection quota policy.

| Scenario | Target requests | Header | Raw response |
| --- | ---: | --- | --- |
| Expired token; refresh rejected | 0 | `notSent` | AuthProxy 500 error |
| Expired token; refresh succeeds; target quota exhausted | 0 | `notSent` | Synthetic 429 |
| Target 401; refresh; successful replay | 2 | `sent` | Replayed upstream status, headers, and body |
| Target 401; refresh; replay connection dropped | 2 | `sent` | AuthProxy 500 error |
| Target 401; refresh; replay quota exhausted | 1 | `sent` | Synthetic 429 |

Every assertion reads `X-AuthProxy-Send-Evidence` from the actual HTTP response.
AuthProxy error bodies retain their ordinary JSON shape. A dropped replay keeps
the existing raw error behavior; it does not return the discarded first 401.

Requests are bodyless POSTs: the existing raw rules allow one OAuth recovery
replay, while `net/http` will not silently retry the POST after a connection
drop. Resource wildcard scripts stay sequential and are cleared by the provider
fixture's cleanup. Unique query markers select provider recordings without
depending on container clock alignment.

Run the focused coverage from `integration_tests`:

```sh
AUTH_PROXY_TEST_DATABASE_PROVIDER=sqlite go test -tags integration ./oauth2 -run '^TestRawProxySendEvidence_' -count=1
AUTH_PROXY_TEST_DATABASE_PROVIDER=postgres go test -tags integration ./oauth2 -run '^TestRawProxySendEvidence_' -count=1
```

See [the proxying guide](../../docs/src/content/docs/sdks/proxying.md) for the
public contract and conservative treatment of missing evidence.
