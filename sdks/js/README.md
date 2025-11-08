# @authproxy/api

A lightweight JavaScript/TypeScript SDK for interacting with AuthProxy APIs.

- Written in TypeScript
- Axios-based HTTP client with XSRF handling
- Tree-shakeable ESM build with type declarations

## Installation

When developing inside this monorepo, you typically do NOT install this package from a registry.

- For local development with the existing UI apps, use the Vite + TypeScript path alias approach described below.
- If you want to build and publish this as a standalone NPM package, see the Build/Publish section.

## Usage

Configure the client once at app startup (baseURL is required):

```ts
import { configureClient, connectors, connections } from '@authproxy/api';

configureClient({
  baseURL: import.meta.env.VITE_PUBLIC_BASE_URL, // or VITE_ADMIN_BASE_URL
});

// Example calls
const list = await connectors.list({ limit: 20 });
const conn = await connections.get('uuid');
```

### XSRF handling

- The SDK automatically captures the `X-XSRF-TOKEN` header returned by the server and sends it on subsequent requests.
- To exclude sending the token for certain endpoints (e.g., session initiation), provide `xsrfExcludePaths` when configuring the client:

```ts
configureClient({
  baseURL: "http://localhost:8081",
  xsrfExcludePaths: ["/api/v1/session/_initiate"],
});
```

### API surface

The SDK exports modules mirroring the server endpoints:

- Actors: `listActors`, `getActorById`, `getActorByExternalId`, `getMe`, `deleteActorById`, `deleteActorByExternalId`
- Connectors: `listConnectors`, `getConnector`
- Connections: `listConnections`, `getConnection`, `initiateConnection`, `disconnectConnection`, `forceConnectionState`
- Request Log: `listRequests`, `getRequest`
- Session: `session.initiate`, `session.terminate`, plus `isInitiateSessionSuccessResponse`
- Tasks: `getTask`, `pollForTaskFinalized`

All functions return Axios promises with typed `data`.

## Local development with UI apps (no package manager)

Both `ui/admin` and `ui/marketplace` are configured to reference this library directly from source using Vite alias + TS path mapping.

- Vite alias added (already committed):
  - `@authproxy/api -> ../../lib/js/src`
- TS `paths` mapping added (already committed):
  - `@authproxy/api -> ../../lib/js/src`
- Each UI's `src/api/index.ts` now configures the SDK with the appropriate baseURL and re-exports the SDK symbols. This allows existing code importing from `../api` to keep working without further changes.

If you add new exports to the SDK, they will be available immediately in the UIs thanks to the alias.

## Build / Publish (optional)

To produce a distributable package (ESM + types):

```bash
cd lib/js
npm install
npm run build
```

This will emit `dist/` with compiled JS and type declarations. You can publish to a registry as needed.

If you want UIs to consume the built package locally without a registry, you can also add a `file:` dependency in the UI `package.json` (not required for in-repo dev with aliases):

```json
{
  "dependencies": {
    "@authproxy/api": "file:../../lib/js"
  }
}
```

## Notes

- The SDK makes no assumptions about `import.meta.env`; you must configure `baseURL` via `configureClient` before use.
- Axios is a runtime dependency; the UI apps already include it, which satisfies module resolution when importing from source.
