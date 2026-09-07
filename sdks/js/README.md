# @authproxy/api

Package source for AuthProxy's JavaScript/TypeScript SDK. The canonical usage guide, including authentication and proxy examples, is in [the SDK documentation](../../docs/src/content/docs/sdks/javascript.md).

## Contract model

The SDK contains handwritten types for the API's `authproxy.net/v1alpha1`
contract. Managed resources and read-only operational projections use
Kubernetes-style envelopes with `apiVersion`, `kind`, `metadata`, `spec`, and,
where applicable, server-owned `status`. List results use `<Kind>List` with
pagination under `metadata`; imperative operations use typed action envelopes.

Shared transport primitives and constructors live in `src/common.ts`. Resource
modules own their resource-specific metadata, spec, status, patch, list, and
action types alongside their endpoint functions. Do not add flat compatibility
models when the server contract changes—update the canonical SDK type and its
serialization tests together.

Connector generations are not a separate resource type. Each generation is a
`Connector` with the same `metadata.id` and a distinct
`metadata.generation`. A connector reference without a generation resolves to
the primary generation; connection bindings and migration targets use an exact
generation.

Actor signing keys and key provider configuration are write-only inputs. Actor
responses omit signing material, and key responses contain only the server's
redacted provider configuration plus `status.keyDataConfigured`. The SDK types
and fixtures preserve these boundaries but do not perform client-side secret
redaction. `createActorClaim` and `actorResourceToClaim` build the restricted
Actor shape allowed inside AuthProxy JWTs; the helpers deliberately omit
database identity, timestamps, status, and signing material.

## Development

Build from the repository root:

```bash
yarn workspace @authproxy/api build
```

Run the package checks with:

```bash
yarn workspace @authproxy/api typecheck
yarn workspace @authproxy/api test
yarn workspace @authproxy/api lint
```
