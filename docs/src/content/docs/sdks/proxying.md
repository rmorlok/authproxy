---
title: Proxying Requests
---

AuthProxy exposes two ways to send an HTTP request through a connection. Both authorize the caller with the `connections:proxy` permission, apply the connection's credentials, enforce matching rate limits, and record request-event and telemetry data.

| Endpoint | Body handling | Response handling | Best for |
|---|---|---|---|
| `POST /api/v1/connections/{id}/_proxy` | JSON envelope; buffered | JSON envelope; buffered | Ordinary API calls and typed application code |
| `ANY /api/v1/connections/{id}/_proxy_raw` | Original body streams through | Original response streams through | Large files, chunked bodies, and server-sent events |

In both cases, the `Authorization` credential presented to AuthProxy identifies the AuthProxy caller. AuthProxy does not forward that credential upstream; it applies authentication from the selected connection.

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
    "body_json": {
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
| `body_json` | JSON request body |
| `body_raw` | Base64-encoded request bytes; use instead of `body_json` |

`POST`, `PUT`, and `PATCH` requests require either `body_json` or `body_raw`. Do not set both.

AuthProxy returns the upstream result in a response envelope:

```json
{
  "status_code": 201,
  "headers": {
    "Content-Type": "application/json"
  },
  "body_json": {
    "id": "widget-123",
    "name": "example"
  }
}
```

The outer AuthProxy request returns `200` when an upstream response was received. Inspect `status_code` for the third-party status. AuthProxy authorization, validation, or transport failures use an error status on the outer request.

Responses with an `application/json` content type populate `body_json`. Other response bodies are returned as base64-encoded `body_raw`.

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
  body_json: {name: 'example'},
};

const {data} = await client.post<ProxyResponse>(
  `/api/v1/connections/${encodeURIComponent(connectionId)}/_proxy`,
  request,
);

if (data.status_code >= 400) {
  throw new Error(`Upstream returned ${data.status_code}`);
}
```

See the [JavaScript/TypeScript SDK guide](/sdks/javascript/) for client configuration and session handling.

## Streaming raw proxy

The raw endpoint preserves streaming semantics in both directions. Send the upstream URL in `X-AuthProxy-Upstream-URL`; the HTTP method, body, and ordinary headers become the upstream request.

```bash
curl --no-buffer \
  "$AUTHPROXY_API_URL/api/v1/connections/$CONNECTION_ID/_proxy_raw" \
  --header "Authorization: Bearer $TOKEN" \
  --header "X-AuthProxy-Upstream-URL: https://api.example.com/v1/events" \
  --header "Accept: text/event-stream" \
  --header "X-AuthProxy-Label: app.example.com/tenant=tenant-42"
```

Requirements and behavior:

- `X-AuthProxy-Upstream-URL` must be an absolute `http` or `https` URL.
- Repeat `X-AuthProxy-Label: key=value` for multiple request labels.
- AuthProxy strips its envelope headers, the caller's `Authorization` header, and hop-by-hop headers before forwarding.
- The upstream response status, headers, body, and trailers stream back to the caller.

### Use `ap proxy`

The `ap` CLI signs the AuthProxy request and handles the raw-proxy envelope. One-shot mode is the shortest path for `curl` or `wget`:

```bash
ap proxy --connection cxn_abc curl https://api.example.com/v1/widgets

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
