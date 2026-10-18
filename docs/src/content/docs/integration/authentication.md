---
title: Authenticate requests to AuthProxy
description: Sign the JWTs your backend uses to call AuthProxy and to open Marketplace sessions for your users.
---

AuthProxy does not have its own login for your users. Instead, your backend
signs a JSON Web Token (JWT) that says "this request is from user `usr_7` in
tenant `acme_42`", and AuthProxy verifies the signature with a public key you
configure. This page explains how to set that up and what goes in the token.

You need this for two things:

- **Backend API calls**, such as creating connections or proxying requests.
  Your backend sends the JWT in an `Authorization: Bearer` header.
- **Opening the Marketplace** for a signed-in user. Your backend signs a
  one-time JWT and redirects the browser to the Marketplace, which exchanges
  it for a session cookie. See [Embed the
  Marketplace](/integration/marketplace/).

## Configure the trusted key

Generate a key pair and keep the private key in your backend's secret store:

```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out host-signing.pem
openssl pkey -in host-signing.pem -pubout -out host-signing.pub
```

Give AuthProxy only the public key, under `systemAuth.jwtSigningKey`:

```yaml
systemAuth:
  jwtSigningKey:
    publicKey:
      path: /etc/authproxy/host-signing.pub
```

AuthProxy accepts RSA (`RS256`), ECDSA (`ES256`, `ES384`, `ES512`), and
Ed25519 (`EdDSA`) public keys. It can also verify `HS256` tokens with a shared
secret configured as `sharedKey`, but an asymmetric key is better because
AuthProxy then holds nothing that can mint tokens.

Anyone holding this private key can sign tokens for any actor, with that
actor's permissions, and can create new actors through the `actor` claim
described below. Treat it like a database root password.

## Token claims

| Claim | Required | Meaning |
|---|---|---|
| `sub` | Yes | The actor's external ID, normally your immutable user or service ID. |
| `namespace` | No | The namespace the actor lives in. Defaults to `root`. |
| `aud` | Yes | The AuthProxy service the token is for: `api`, `public`, or `admin-api`. A token is rejected by any service not listed. |
| `exp` | Recommended | Expiry time. Keep it to a few minutes. Required when `nonce` is set. |
| `nonce` | For handoffs | A one-time value starting with `non_`. AuthProxy rejects a second use of the same nonce. Use it on every Marketplace handoff token. |
| `actor` | No | A full actor definition. If present, AuthProxy creates or updates the actor from it. See below. |
| `permissions` | No | Narrows what this token can do. Must be a subset of the actor's permissions. |

`sub` and `namespace` together identify the actor. If the token has no
`actor` claim, an actor with that external ID must already exist in that
namespace, or AuthProxy returns `401 actor does not exist`.

## Sign a token

This Node.js example uses the [`jose`](https://github.com/panva/jose) library.
Any JWT library works.

```ts
import {SignJWT, importPKCS8} from 'jose';
import {randomBytes} from 'node:crypto';

const privateKey = await importPKCS8(process.env.AUTHPROXY_SIGNING_KEY_PEM!, 'RS256');

// A token for your backend to call the API service on behalf of a user.
export async function apiToken(userId: string, tenantId: string) {
  return new SignJWT({namespace: `root.tenants.${tenantId}`})
    .setProtectedHeader({alg: 'RS256'})
    .setSubject(userId)
    .setAudience('api')
    .setIssuedAt()
    .setExpirationTime('5m')
    .sign(privateKey);
}

// A one-time token for opening the Marketplace as a user.
export async function marketplaceHandoffToken(userId: string, tenantId: string) {
  return new SignJWT({
    namespace: `root.tenants.${tenantId}`,
    nonce: `non_${randomBytes(16).toString('hex')}`,
  })
    .setProtectedHeader({alg: 'RS256'})
    .setSubject(userId)
    .setAudience('public')
    .setIssuedAt()
    .setExpirationTime('2m')
    .sign(privateKey);
}
```

Send the API token as a bearer token:

```bash
curl https://authproxy-api.internal.example.com/api/v1/connections \
  -H "Authorization: Bearer $TOKEN"
```

## Creating actors: ahead of time or on first use

There are two ways for actors to come into existence.

**Provision ahead of time.** Your backend creates and updates actors through
the API or with `ap apply` whenever users join, leave, or change role. Tokens
then carry only `sub` and `namespace`. This keeps permissions in one place
that you can audit.

**Provision on first use.** The token carries the full actor in an `actor`
claim, and AuthProxy creates or updates the actor each time it sees the
token. This removes the sync job, but it means whatever signs tokens also
decides permissions.

```json
{
  "sub": "usr_7",
  "namespace": "root.tenants.acme_42",
  "aud": "api",
  "exp": 1760000000,
  "actor": {
    "apiVersion": "authproxy.net/v1alpha1",
    "kind": "Actor",
    "metadata": {
      "namespace": "root.tenants.acme_42",
      "labels": {"acme.com/role": "member"}
    },
    "spec": {
      "externalId": "usr_7",
      "permissions": [
        {
          "namespace": "root.tenants.acme_42.**",
          "resources": ["connectors"],
          "verbs": ["list", "get"]
        },
        {
          "namespace": "root.tenants.acme_42.**",
          "resources": ["connections"],
          "verbs": ["create", "list", "get", "proxy"]
        }
      ]
    }
  }
}
```

`sub` must equal `actor.spec.externalId`, and `namespace` must equal
`actor.metadata.namespace`. The `actor` claim may not contain server-managed
fields such as `metadata.id`, `status`, or a signing key. See [Map users and
tenants](/integration/host-application/#provision-actors) for how to choose
between the two approaches.

## Narrowing a token

An actor's permissions describe everything it may ever do. A single token
can carry a smaller set in its `permissions` claim. For example, a sync job
acting for a user might only need to proxy through one connection:

```json
{
  "sub": "usr_7",
  "namespace": "root.tenants.acme_42",
  "aud": "api",
  "exp": 1760000300,
  "permissions": [
    {
      "namespace": "root.tenants.acme_42",
      "resources": ["connections"],
      "resourceIds": ["cxn_Z3tMSSzTDa8piyM1"],
      "verbs": ["proxy"]
    }
  ]
}
```

AuthProxy allows a request only if both the actor's permissions and the
token's permissions allow it. A token that asks for more than the actor has is
rejected outright rather than trimmed. The full list of resources and verbs is
in [Permission resources and verbs](/security/permission-resources-and-verbs/).

## Actors with their own keys

An actor can also be given its own public key. Tokens for that actor can then
be signed with the actor's private key as well as with the host key. This
suits automation and operators, where each identity should hold a separate
credential. The `ap` CLI works this way: in the local development
environment, the `bobdole` actor's key pair is in `dev_config/keys/admin/`.

You can configure such actors in `systemAuth.actors` by pointing at a
directory of `.pub` files, or set a key on an actor through the API. See the
[configuration reference](/reference/configuration/#configured-actor-namespaces).

## When a token is rejected

AuthProxy returns `401` with a short reason. The common ones:

| Error | Cause |
|---|---|
| `aud '...' not valid for service '...'` | The token's `aud` does not include the service you called. Tokens for `api` do not work on `public`, and so on. |
| `actor does not exist` | No actor with this `sub` exists in `namespace`, and the token has no `actor` claim. |
| `jwt nonce already used` | A handoff token was replayed, or your code reuses nonces. |
| `cannot use nonce in jwt without expiration time` | Add `exp` to any token that has a `nonce`. |
| `expected prefix "non_"` | Nonces must start with `non_`. |
| `token permissions exceed actor permissions` | The `permissions` claim asks for more than the actor has. |

## Security checklist

- Sign tokens only on your backend, after you have authenticated the user.
  Never ship the private key to a browser or mobile app.
- Keep expiry times short and request only the audience you need.
- Use a fresh nonce for every Marketplace handoff.
- Keep handoff URLs, which contain the token, out of logs and analytics.
- Use your own immutable user IDs for `sub`, not email addresses.

The [authentication and authorization](/security/authentication-and-authorization/)
page covers the full trust model, including browser sessions and how
permission templates work.
