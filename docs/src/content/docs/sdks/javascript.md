---
title: JavaScript and TypeScript SDK
---

`@authproxy/api` is the repository's typed JavaScript/TypeScript client for AuthProxy. It uses Axios, exports ESM and TypeScript declarations, and is shared by the Marketplace and Admin UIs.

The package currently lives at [`sdks/js/`](https://github.com/rmorlok/authproxy/tree/main/sdks/js/). Inside this monorepo, Yarn resolves it as the `@authproxy/api` workspace.

## Configure the client

Call `configureClient` once before using any resource module:

```ts
import {configureClient, connections} from '@authproxy/api';

configureClient({
  baseURL: 'http://localhost:8081',
});

const {data} = await connections.list({limit: 20});
console.log(data.results);
```

The shared client defaults to a 10-second timeout, `withCredentials: true`, and an `Accept: application/json` header.

### Browser sessions and XSRF

With cookie-based AuthProxy sessions, keep `withCredentials: true`. The client captures the `X-XSRF-TOKEN` response header and sends the token on later requests. Session initiation is excluded by default because no session exists yet.

```ts
configureClient({
  baseURL: 'https://marketplace.example.com',
  withCredentials: true,
  xsrfExcludePaths: ['/api/v1/session/_initiate'],
});
```

The package also exports `setXsrfToken` and `getXsrfToken` for tests or explicit token management.

### Bearer tokens

For a server process or script, provide an AuthProxy JWT as a default header and disable cookies:

```ts
configureClient({
  baseURL: process.env.AUTHPROXY_API_URL,
  withCredentials: false,
  defaultHeaders: {
    Accept: 'application/json',
    Authorization: `Bearer ${process.env.AUTHPROXY_TOKEN}`,
  },
});
```

Scope the JWT to only the services, namespaces, resources, verbs, and resource IDs the application needs.

## API surface

The package exports the shared Axios `client`, individual functions, typed models, and grouped modules for:

- actors
- connectors and connector versions
- connections and setup flows
- namespaces
- encryption keys
- rate limits and dry runs
- application metrics and request events
- sessions
- background tasks and workflow monitoring

Grouped modules provide concise calls such as `connections.get(id)`, `connectors.list(params)`, and `namespaces.getByPath(path)`. Individual functions such as `getConnection` are exported as well. Calls return Axios promises; the typed AuthProxy payload is in `response.data`.

```ts
import {connections, ConnectionState} from '@authproxy/api';

const {data: connection} = await connections.get('cxn_abc');

if (connection.state === ConnectionState.CONFIGURED) {
  console.log('Connection setup is complete');
}
```

## Send a request through a connection

The SDK exports `ProxyRequest`, `ProxyResponse`, `ProxyMethod`, and the shared `client`, but it does not currently include a dedicated proxy function. Call the wrapped proxy endpoint with `client.post`:

```ts
import {
  client,
  type ProxyRequest,
  type ProxyResponse,
} from '@authproxy/api';

const request: ProxyRequest = {
  url: 'https://api.example.com/v1/widgets',
  method: 'GET',
  headers: {Accept: 'application/json'},
  labels: {'app.example.com/tenant': 'tenant-42'},
};

const {data} = await client.post<ProxyResponse>(
  `/api/v1/connections/${encodeURIComponent(connectionId)}/_proxy`,
  request,
);

console.log(data.status_code, data.body_json);
```

The wrapped endpoint buffers request and response bodies. For large files, chunked bodies, or server-sent events, use [`ap proxy`](/sdks/proxying/#use-ap-proxy) and the streaming raw endpoint instead.

See [Proxying requests](/sdks/proxying/) for the complete request shape, raw-body encoding, response semantics, and direct HTTP examples.

## Use the package in this repository

The Admin and Marketplace UIs import the SDK directly from source. Their Vite aliases and TypeScript path mappings resolve:

```text
@authproxy/api -> ../../sdks/js/src
```

New exports added under `sdks/js/src` are therefore available to those applications without publishing a package.

Build the distributable ESM and declaration files from the repository root:

```bash
yarn install
yarn workspace @authproxy/api build
```

The build writes to `sdks/js/dist/`. The package also provides `typecheck`, `test`, and `lint` workspace scripts.
