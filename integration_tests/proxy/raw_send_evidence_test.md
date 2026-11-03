# Raw proxy send evidence

Companion notes for `raw_send_evidence_test.go`. The canonical contract is in
[Proxying Requests](../../docs/src/content/docs/sdks/proxying.md).

## Approach

Like the wrapped suite, these tests use production API routes on a real HTTP
listener and a separate `helpers.ControlledUpstream`. Both hops use real TCP;
AuthProxy runs in the test process with SQLite or PostgreSQL and real Redis
rate limiting. The upstream independently records the method, headers, and
complete request body before responding or dropping a connection.

Tests read the public `X-AuthProxy-Send-Evidence` header independently of the
runtime constant. Raw AuthProxy error JSON must keep its previous shape without
a `sendEvidence` member. Normal raw provider statuses and bodies pass through.

## Coverage

| Scenario | Expected result |
|---|---|
| Provider 200, 401, 403, 429, and 500 | Original status/body and `sent`; one recorded POST. |
| Provider 204 and HEAD responses with status 200/204 | Empty body and `sent`; header commitment does not require a body write. |
| Provider sends malformed JSON with JSON content type | Opaque bytes pass through with `sent`; raw forwarding does not decode JSON. |
| Missing or invalid authentication, permission denial, invalid envelope/connection | Original error status/JSON and `notSent`; no target request. |
| Provider 429 then local backoff | First `sent`, then `notSent`; the second request never reaches upstream. |
| Full POST received, then socket closed before headers | Error response with `mayHaveSent`; upstream recording proves the operation arrived. |
| Response headers followed by truncated body | Committed `sent` remains present and body consumption fails with unexpected EOF. |
| Redirect followed by connection drop | Final error retains `sent` from the earlier redirect response. |
| Redirect followed by local backoff | Final synthetic 429 retains `sent`; the redirected hop never reaches upstream. |
| Caller-provided reserved header, including duplicate values | Removed before target dispatch. |
| Provider reserved header, declared trailer, or undeclared late trailer | Cannot override the single trusted header; reserved trailer declarations and values are removed. |
| Unrelated provider header, body, and trailers | Preserved. |
| SSE with response headers flushed before any event | Client receives `sent` while the first body byte remains blocked upstream. |
| Provider sends response headers before upload finishes | Caller receives `sent` before releasing the remaining upload; all request/response bytes survive and request capture includes the complete upload. |
| Sequential and overlapping calls through one connection | Each request retains its own evidence. |
| API and proxy-enabled public service with CORS | Success/error headers expose evidence and configured fields; an upstream exposure list is merged rather than replacing them. |

## Determinism and limits

SSE coverage gates the first event on a channel and requires `http.Client.Do`
to return the response headers before releasing it. It therefore detects header
buffering even when an implementation would flush after the first body write.
Concurrency tests similarly gate target responses until independent upstream
receipt is observed. Client deadlines bound cleanup; they are not the failure
injection mechanism.

The response-before-upload-completion case sends a 64 KiB body through a pipe
with both known-length and chunked framing, withholding its tail until the caller
receives response headers.
The provider enables HTTP/1 full duplex, reads the prefix, and flushes headers
before reading the tail. This catches the server's automatic request-body drain
when an early proxy flush is attempted without full-duplex support. The test also
waits for the persisted full log and verifies capture includes the entire upload,
so returning headers early cannot silently truncate the recorded request body.
The chunked variant retains the streaming capture-skip reason but records the
complete request byte count after the upload ends.

Direct drop scenarios use POST bodies. The redirect/drop scenario closes the
redirect connection before following the redirected GET, preventing a reused
socket from triggering Go's transparent retry and obscuring request counts.

No assertion equates send evidence with successful provider-side execution or
safe retryability. A disconnected downstream caller cannot inspect a response;
lower-level tests cover cancellation and panic tracking. OAuth recovery is
covered by its companion integration suite. The integration
README contains focused commands for both database providers.
