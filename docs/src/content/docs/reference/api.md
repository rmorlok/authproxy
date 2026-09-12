---
title: API reference
---

AuthProxy exposes separate application-facing and administrative HTTP APIs.
Both publish generated Swagger 2.0 documentation when the service is running.
For the common envelope and examples of every managed kind, start with
[Resource contracts](/reference/resources/).

| API | Local Swagger UI | Checked-in JSON |
|---|---|---|
| Application API | `http://localhost:8081/swagger/index.html` | [`internal/service/api/swagger/docs.json`](https://github.com/rmorlok/authproxy/blob/main/internal/service/api/swagger/docs.json) |
| Admin API | `https://localhost:8082/swagger/index.html` | [`internal/service/admin_api/swagger/docs.json`](https://github.com/rmorlok/authproxy/blob/main/internal/service/admin_api/swagger/docs.json) |

The Admin URL uses the self-signed development certificate in a local checkout.
Production URLs depend on deployment routing.

:::caution[Breaking contract cutover]

All AuthProxy-owned JSON fields, YAML fields, and URL query/path parameter names
use lowerCamelCase. Snake_case input is rejected; there are no aliases or
migration window. Reset persisted AuthProxy state before deploying this release
instead of starting it against old serialized connector or configuration data.

:::

## Which API to use

- Use the **application API** for host-application resource access and
  connection-scoped proxy requests.
- Use the **Admin API** for operator workflows and broader management
  endpoints.
- Use the **public service** for Marketplace sessions and OAuth browser
  callbacks; its browser-oriented routes are not a substitute for the
  application API.

Every protected request needs an actor JWT or a browser session with the
required namespace, resource, resource-ID, and verb scope. See
[authentication and authorization](/security/authentication-and-authorization/).

## Resource identity and names

Namespace, actor, connector, connection, key, and rate-limit responses expose a
human-readable `metadata.name` alongside their direct identity (`metadata.id`,
or the canonical path for a namespace). Keep using the immutable ID in URLs,
permissions, foreign-key fields, and stored references. Names are for display
and discovery; renaming a resource does not change its URL.

Create requests for actors, connectors, keys, and rate limits may include
`metadata.name`; Connection initiation may include `spec.name`. If omitted,
AuthProxy generates the ID first and uses that ID as the initial name:

```http
POST /api/v1/connections/_initiate
Content-Type: application/json

{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "ConnectionInitiate",
  "metadata": {
    "target": {
      "apiVersion": "authproxy.net/v1alpha1",
      "kind": "Connector",
      "id": "cxr_01example",
      "generation": 3
    }
  },
  "spec": {
    "intoNamespace": "root.acme",
    "name": "production-crm",
    "returnToUrl": "https://app.example.com/integrations/complete"
  }
}
```

Omit the connector generation to select its primary generation. The setup
result is a `ConnectionSetup` action whose `metadata.target` identifies the new
connection and whose `status.type` is `redirect`, `form`, `verifying`,
`complete`, or `error`.

`intoNamespace` is optional. AuthProxy first tries the connector's namespace.
If the actor cannot create connections there but has `connections:create` in
exactly one permitted child namespace, AuthProxy uses that unambiguous child.
It never infers from wildcard permissions or from multiple candidate
namespaces; pass `intoNamespace` explicitly in those cases.

Rename through the immutable ID. The response keeps the same `id` and returns
the new `name`:

```http
PATCH /api/v1/connections/cxn_01example
Content-Type: application/json

{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "Connection",
  "metadata": {
    "name": "production-salesforce"
  },
  "spec": {}
}
```

Names are case-sensitive and unique among live resources of the same type in
the same namespace. A conflicting create or rename returns `409 Conflict` and
does not expose a database constraint. Deleting a resource releases its name
for reuse. The same name may appear on another resource type or in another
namespace.

A connector has one name shared by all definition generations. Rename it with
`PATCH /api/v1/connectors/{connectorId}`. Generation-specific create and update
requests do not accept a separate name. A namespace name is read-only and is
derived from the final segment of its path.

Labels and annotations are also updated only through the parent resource's
`metadata` patch. The former `/labels`, `/annotations`, and namespace `/key`
subroutes do not exist in the resource contract.

### Query by name

Collection APIs accept an exact `name` filter in addition to namespace and
label filters:

```http
GET /api/v1/connections?name=production-salesforce&namespace=root.acme
```

The namespace restriction is important when a query can span multiple
namespaces, because those namespaces may contain the same name. Results still
include both `metadata.name` and immutable `metadata.id`.

## Connection resource shape

Connection reads and lists use the `authproxy.net/v1alpha1` resource envelope.
The connector binding is generation-specific because stored configuration and
credentials are interpreted by that exact connector definition. Connections
are namespace-scoped and are not bound to an individual actor.

```json
{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "Connection",
  "metadata": {
    "id": "cxn_01example",
    "name": "production-salesforce",
    "namespace": "root.acme"
  },
  "spec": {
    "connectorRef": {
      "apiVersion": "authproxy.net/v1alpha1",
      "kind": "Connector",
      "id": "cxr_01example",
      "generation": 3
    },
    "configuration": {"tenant": "acme"}
  },
  "status": {
    "lifecycle": {"state": "configured"},
    "health": {"state": "healthy"},
    "configuration": {
      "configured": true,
      "schema": {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "type": "object",
        "required": ["tenant"],
        "properties": {
          "tenant": {"type": "string", "minLength": 1}
        },
        "additionalProperties": true
      }
    }
  }
}
```

Submit setup values through the typed setup actions. AuthProxy validates them
against the connector's form definition and encrypts stored values. Connection
reads return decrypted connector-authored values under `spec.configuration`
and report whether that configuration satisfies its aggregate schema under
`status.configuration.configured`. The server-derived aggregate JSON Schema is
under `status.configuration.schema`; clients cannot set it on a Connection.
The aggregate retains `required` rules from unconditional setup steps. It omits
requirements from conditionally eligible steps because their JavaScript
predicates cannot be represented faithfully by the aggregate JSON Schema;
setup submissions still use each step's original schema for validation.

The aggregate permits additional properties because connector migration hooks
can create configuration fields without a form schema. Those fields are
returned in `spec.configuration`, but remain untyped in the aggregate schema.

Auth-method credentials do not appear in either field. API keys, OAuth client
credentials, access tokens, and refresh tokens remain in dedicated encrypted
credential storage and are never returned through the Connection resource.
Connector authors should use those auth methods for secret material rather
than collecting it in custom setup fields. A caller authorized to get or list
a Connection can read its connector-authored configuration.

The Admin cross-resource endpoint searches names directly and also searches
user-label values:

```http
GET /api/v1/search/resources?q=production&kind=Connection
```

Exact name matches rank before name prefixes, which rank before name
substrings and matching label values. Namespace and resource-ID permissions are
applied before results are returned. Search results are heterogeneous
projections, not partial resource objects. Each item carries one typed
`resourceRef`; partial-result observations live in the list metadata:

```json
{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "SearchResultList",
  "metadata": {
    "truncatedKinds": ["Connection"],
    "incompleteKinds": []
  },
  "items": [
    {
      "resourceRef": {
        "apiVersion": "authproxy.net/v1alpha1",
        "kind": "Connection",
        "id": "cxn_01example",
        "name": "production-salesforce",
        "namespace": "root.acme"
      },
      "labels": {"team": "payments"},
      "matchedLabels": [],
      "updatedAt": "2026-08-16T12:00:00Z"
    }
  ]
}
```

Namespace results use the immutable namespace path as an ID-only reference;
clients can derive the display segment from that path. Search never returns
request events, task history, or other operational projections as if they were
durable resources. See [connector migration notifications](/operations/connector-generation-migrations/#notifications)
for the separate actor-specific notification projection and its view actions.

## Actor resource shape

Actor create, read, update, and list operations use the
`authproxy.net/v1alpha1` resource envelope. Namespace and identity fields live
in `metadata`; the host identity and permissions live in `spec`:

```http
POST /api/v1/actors
Content-Type: application/json

{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "Actor",
  "metadata": {
    "namespace": "root.acme",
    "name": "billing-service",
    "labels": {"team": "payments"}
  },
  "spec": {
    "externalId": "svc_billing",
    "permissions": [
      {
        "namespace": "root.acme.**",
        "resources": ["connections"],
        "verbs": ["list", "get", "proxy"]
      }
    ]
  }
}
```

Updates use the same type metadata and require both `metadata` and `spec`,
even when one section has no changes:

```json
{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "Actor",
  "metadata": {"labels": {"team": "platform"}},
  "spec": {}
}
```

`spec.signingKey` is write-only. AuthProxy encrypts supplied signing material
and reports only `status.signingKeyConfigured` in returned resources. An
explicit `"signingKey": null` in an update removes the actor-specific key.

## Regenerate specifications

Swagger artifacts are generated from Go route annotations. Run:

```bash
./scripts/generate-swagger.sh
```

The repository preflight runs the same generation and fails if the checked-in
artifacts are stale:

```bash
./scripts/preflight.sh
```

For task-oriented request examples, start with
[proxying requests](/sdks/proxying/) and the
[JavaScript SDK](/sdks/javascript/).
