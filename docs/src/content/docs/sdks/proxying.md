---
title: Proxying Requests
---

AuthProxy exposes two ways to send an HTTP request through a connection. Both authorize the caller with the `connections:proxy` permission, apply the connection's credentials, enforce matching rate limits, and record request-event and telemetry data.

| Endpoint | Body handling | Response handling | Best for |
|---|---|---|---|
| `POST /api/v1/connections/{id}/_proxy` | JSON envelope; buffered | JSON envelope; buffered | Ordinary API calls and typed application code |
| `ANY /api/v1/connections/{id}/_proxyRaw` | Original body streams through | Original response streams through | Large files, chunked bodies, and server-sent events |

In both cases, the `Authorization` credential presented to AuthProxy identifies the AuthProxy caller. AuthProxy does not forward that credential upstream; it applies authentication from the selected connection.

Proxy URLs always use the immutable connection ID. A connection name is useful
for display and exact-name discovery, but renaming it does not change either
proxy endpoint or a stored `$CONNECTION_ID` reference.

## Wrapped proxy

The wrapped endpoint accepts a JSON description of the upstream request:

```bash
curl --request POST \
  "$AUTHPROXY_API_URL/api/v1/connections/$CONNECTION_ID/_proxy" \
  --header "Authorization: Bearer $TOKEN" \
  --header "Content-Type: application/json" \
  --data '{
    "url": "https://api.example.com/v1/widgets",
    "method": "POST",
    "headers": {
      "Content-Type": "application/json",
      "Accept": "application/json"
    },
    "labels": {
      "app.example.com/tenant": "tenant-42"
    },
    "bodyJson": {
      "name": "example"
    }
  }'
```

The request fields are:

| Field | Meaning |
|---|---|
| `url` | Absolute upstream URL |
| `method` | HTTP method such as `GET`, `POST`, or `PATCH` |
| `headers` | Headers to send upstream, excluding provider credentials |
| `labels` | Optional request labels used by rate limits, request events, and telemetry |
| `bodyJson` | JSON request body |
| `bodyRaw` | Base64-encoded request bytes; use instead of `bodyJson` |

`POST`, `PUT`, and `PATCH` requests require either `bodyJson` or `bodyRaw`. Do not set both.

AuthProxy returns the upstream result in a response envelope:

```json
{
  "statusCode": 201,
  "sendEvidence": "sent",
  "headers": {
    "Content-Type": "application/json"
  },
  "bodyRaw": null,
  "bodyJson": {
    "id": "widget-123",
    "name": "example"
  }
}
```

The outer AuthProxy request returns `200` when the proxy produces a response envelope, including a locally generated rate-limit response. Inspect `statusCode` for that result's HTTP status. AuthProxy authorization, validation, or transport failures use an error status on the outer request.

Responses with an `application/json` content type populate `bodyJson`. Other response bodies are returned as base64-encoded `bodyRaw`.

### Target-send evidence

The wrapped endpoint includes `sendEvidence` in its response envelope and in
AuthProxy error bodies. The raw endpoint carries the same evidence in the
`X-AuthProxy-Send-Evidence` response header. It describes all target attempts
made for that logical request, including redirects and a retry after
credential recovery:

| Value | Meaning |
|---|---|
| `notSent` | AuthProxy can prove that no target attempt reached the underlying HTTP transport. Examples include validation or authorization rejection, credential resolution failure, and a local rate-limit rejection before any target send. |
| `mayHaveSent` | An attempt reached the target transport without a confirmed HTTP response, or tracking is incomplete. DNS/TLS failures and a lost response fall here; the provider may still have performed the action. |
| `sent` | At least one target attempt returned an HTTP response, regardless of status. This includes provider 401, 429, and 500 responses. It does not prove that the requested action succeeded. |

A local 429 can carry `notSent`, while a provider 429 carries `sent`. If the
provider first returns 401 and a credential retry is later rejected locally,
the result remains `sent`. OAuth token requests are excluded: a failed token
refresh can leave the target request `notSent` even though AuthProxy contacted
the token endpoint. A response-body or JSON-decoding failure after a target
response also retains `sent`.

For example, a wrapped-proxy validation error returns an outer `400` with:

```json
{
  "error": "url is required",
  "sendEvidence": "notSent"
}
```

Only `notSent` rules out a target send. Decide whether and when to retry based
on the failure cause, backoff requirements, and the provider's idempotency
contract. Neither `sent` nor `mayHaveSent` makes a non-idempotent request safe
to repeat automatically.

Treat a missing or unrecognized field or header as unknown. Older servers,
global middleware rejections before proxy routing, panic recovery, and
infrastructure-generated errors may omit evidence; a disconnected caller may
receive no response at all. None of these outcomes proves `notSent`.

### JavaScript

The JavaScript SDK exports the shared Axios `client` and the `ProxyRequest` and `ProxyResponse` types. It does not currently export a dedicated proxy function:

```ts
import {
  client,
  configureClient,
  type ProxyRequest,
  type ProxyResponse,
} from '@authproxy/api';

configureClient({
  baseURL: 'http://localhost:8081',
  withCredentials: false,
  defaultHeaders: {
    Accept: 'application/json',
    Authorization: `Bearer ${token}`,
  },
});

const request: ProxyRequest = {
  url: 'https://api.example.com/v1/widgets',
  method: 'POST',
  headers: {'Content-Type': 'application/json'},
  bodyJson: {name: 'example'},
};

const {data} = await client.post<ProxyResponse>(
  `/api/v1/connections/${encodeURIComponent(connectionId)}/_proxy`,
  request,
);

if (data.statusCode >= 400) {
  throw new Error(`Upstream returned ${data.statusCode}`);
}

console.log(data.sendEvidence ?? 'mayHaveSent');
```

See the [JavaScript/TypeScript SDK guide](/sdks/javascript/) for client configuration and session handling.

## Streaming raw proxy

The raw endpoint preserves streaming semantics in both directions. Send the upstream URL in `X-AuthProxy-Upstream-URL`; the HTTP method, body, and ordinary headers become the upstream request.

```bash
curl --include --no-buffer \
  "$AUTHPROXY_API_URL/api/v1/connections/$CONNECTION_ID/_proxyRaw" \
  --header "Authorization: Bearer $TOKEN" \
  --header "X-AuthProxy-Upstream-URL: https://api.example.com/v1/events" \
  --header "Accept: text/event-stream" \
  --header "X-AuthProxy-Label: app.example.com/tenant=tenant-42"
```

Requirements and behavior:

- `X-AuthProxy-Upstream-URL` must be an absolute `http` or `https` URL.
- Repeat `X-AuthProxy-Label: key=value` for multiple request labels.
- AuthProxy strips its envelope headers, the caller's `Authorization` header, and hop-by-hop headers before forwarding.
- The upstream response status, body, and ordinary headers and trailers stream back to the caller.
- AuthProxy reserves `X-AuthProxy-Send-Evidence`: caller and upstream headers or trailers cannot override its value.

### Read evidence on raw responses

Ordinary raw-proxy responses, including AuthProxy errors, carry exactly one
`X-AuthProxy-Send-Evidence` header with `notSent`, `mayHaveSent`, or `sent`.
These values have the same [target-send meaning](#target-send-evidence) as the
wrapped field. Raw AuthProxy error JSON keeps its existing shape; read the
header for evidence.

The `--include` (`-i`) flag above prints response headers. For example, a target
SSE response can begin with:

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
X-AuthProxy-Send-Evidence: sent
```

AuthProxy commits the evidence header before streaming the body. For SSE and
other streams, `sent` confirms that a target response was observed; it does not
confirm stream completion. A later truncated body, read failure, or caller
disconnect cannot revise the header already delivered.

When CORS is enabled, the API service and a proxy-enabled public service expose
this header to browser code through `Access-Control-Expose-Headers`, alongside
configured exposed headers. This does not add allowed origins, methods, or
request headers. Missing or unrecognized evidence remains unknown, including
responses from older servers or failures outside the proxy handler.

### Use `ap proxy`

The `ap` CLI signs the AuthProxy request and handles the raw-proxy envelope. One-shot mode is the shortest path for `curl` or `wget`:

```bash
ap proxy --connection cxn_abc curl -i https://api.example.com/v1/widgets

ap proxy --connection cxn_abc curl -N \
  https://api.example.com/v1/events \
  -H 'Accept: text/event-stream'

ap proxy --connection cxn_abc wget \
  https://api.example.com/files/archive.zip \
  -O archive.zip
```

For a long-running local listener, provide an upstream base URL. The incoming path and query are appended to it:

```bash
ap proxy --connection cxn_abc --upstream-base https://api.example.com
curl http://127.0.0.1:9999/v1/widgets
```

All `ap proxy` flags must appear before the literal `curl` or `wget`. See the [`ap` CLI reference](/development/cli/#ap-proxy--connection-scoped-streaming-proxy) for configuration and more recipes.

## Which endpoint should I use?

Use the wrapped endpoint by default. Its explicit request and response types are easier to validate, log, and consume in application code. Choose the raw endpoint when buffering would change behavior or consume too much memory, including uploads, downloads, chunked requests, and live event streams.
