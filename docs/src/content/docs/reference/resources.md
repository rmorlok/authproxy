---
title: Resource contracts
description: Canonical authproxy.net/v1alpha1 resources, lists, references, actions, and read projections.
---

AuthProxy's managed API objects use the Kubernetes-style
`authproxy.net/v1alpha1` contract. This schema version is independent of the
HTTP route prefix: the endpoints remain under `/api/v1`.

## Common envelope

Every managed resource has `apiVersion`, `kind`, `metadata`, and `spec`.
Responses add `status` where the server has observed state to report.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Example
metadata:
  id: ex_01example
  name: example
  namespace: root.acme
  labels: {}
  annotations: {}
  createdAt: 2026-09-07T12:00:00Z
  updatedAt: 2026-09-07T12:00:00Z
spec: {}
status: {}
```

`metadata.id` is the immutable AuthProxy identity. `metadata.name` is mutable,
human-readable identity. Use IDs in route paths and durable references; use
names for display and discovery. Namespace paths use dot-separated IDs such as
`root.acme.payments`.

Create and patch bodies use the same resource kind with operation-specific
field policies. Server-owned IDs, timestamps, and status are rejected on
normal client writes. A patch requires both `metadata` and `spec`; use `{}` for
a section with no changes. Labels and annotations exist only under metadata.

## Lists and references

A homogeneous collection uses `<ItemKind>List`, list metadata, and `items`:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectionList
metadata:
  continue: opaque-next-page-token
items: []
```

References carry type metadata plus either `id`, or `namespace` and `name` when
the referenced resource supports name resolution:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
id: cxr_01example
generation: 3
```

A Connection's connector reference requires an exact generation. References
that intentionally address a logical Connector—such as a RateLimit scope—omit
generation and therefore cover all of its generations.

## Managed resources

### Namespace

A Namespace's immutable `metadata.id` is its full path. Its name is the final
path segment and its namespace is the parent path. Root has no parent.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Namespace
metadata:
  id: root.acme
  name: acme
  namespace: root
spec:
  encryptionKeyRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Key
    id: key_01example
status:
  state: active
```

Assign or clear the encryption key with `PATCH /api/v1/namespaces/{path}` by
setting `spec.encryptionKeyRef` to a Key reference or `null`. There is no
separate namespace-key subresource.

### Key

Key provider configuration is write-only. Reads preserve the resource shape
but redact secret provider fields and report observed lifecycle state in
status.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Key
metadata:
  id: key_01example
  name: tenant-key
  namespace: root.acme
spec:
  usage: data_encryption
  materialType: symmetric
  desiredState: active
  keyData:
    value: "****************"
status:
  state: active
  keyDataConfigured: true
```

### Actor

Actor identity and permissions are desired state. Signing material is
write-only; responses report only whether it is configured.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Actor
metadata:
  id: act_01example
  name: billing-service
  namespace: root.acme
spec:
  externalId: svc_billing
  permissions:
    - namespace: root.acme.**
      resources: [connections]
      verbs: [get, list, proxy]
status:
  signingKeyConfigured: true
```

The `actor` claim in an AuthProxy JWT is a restricted Actor resource. It must
not contain database identity, timestamps, status, or signing material. See
[Authentication and authorization](/security/authentication-and-authorization/).

### Connector and generations

A logical Connector has one immutable `metadata.id` and shared name and
namespace. Every definition snapshot is returned as `kind: Connector`; the
snapshot number is `metadata.generation`. There is no `ConnectorGeneration` kind.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
metadata:
  id: cxr_01example
  name: example-api
  namespace: root.integrations
  generation: 3
  labels:
    provider: example
spec:
  release:
    desiredState: primary
  definition:
    displayName: Example API
    description: Connect to the Example API.
    auth:
      type: api-key
      placement:
        type: bearer
status:
  release:
    state: primary
```

Connector generation endpoints use `generations` consistently in their paths:

- `GET /api/v1/connectors/{id}` resolves the primary generation unless a
  generation is explicitly requested elsewhere.
- `GET /api/v1/connectors/{id}/generations` returns `ConnectorList`.
- `GET /api/v1/connectors/{id}/generations/{generation}` returns one Connector.
- `POST /api/v1/connectors/{id}/generations` creates the next generation.
- `PATCH /api/v1/connectors/{id}/generations/{generation}` updates a draft.

Only canonical `spec.definition` is encrypted in the connector-definition
column. Identity, generation, release state, metadata, and status are not
duplicated inside the encrypted definition. Publishing a new primary moves the
old primary to active; existing Connections remain bound to their exact
generation until explicitly migrated.

Configured connectors use this same YAML shape:

```yaml
connectors:
  loadFromList:
    - apiVersion: authproxy.net/v1alpha1
      kind: Connector
      metadata:
        name: example-api
        namespace: root.integrations
      spec:
        release:
          desiredState: primary
        definition:
          displayName: Example API
          auth:
            type: no-auth
```

### Connection

A Connection binds to an exact Connector generation. Connector-authored,
non-secret setup values are returned in `spec.configuration`; auth-method
credentials remain in dedicated encrypted storage.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connection
metadata:
  id: cxn_01example
  name: production-example
  namespace: root.acme
spec:
  connectorRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connector
    id: cxr_01example
    generation: 3
  configuration:
    tenant: acme
status:
  lifecycle:
    state: configured
  health:
    state: healthy
  configuration:
    configured: true
    schema:
      type: object
      properties:
        tenant:
          type: string
```

Connections are created through the `ConnectionInitiate` action rather than a
plain resource create. Setup, reauthentication, disconnect, and generation
migration are typed actions as well.

### RateLimit

RateLimit scope and matching behavior are desired state; effective mode is
observed status.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: RateLimit
metadata:
  id: rl_01example
  name: example-writes
  namespace: root.acme
spec:
  scope:
    connectorRef:
      apiVersion: authproxy.net/v1alpha1
      kind: Connector
      id: cxr_01example
  mode: enforce
  selector:
    methods: [POST, PUT, PATCH]
  bucket:
    dimensions: [actor]
  algorithm:
    tokenBucket:
      capacity: 60
      refillRate: 1
status:
  effectiveMode: enforce
```

The generationless Connector reference applies the rule across current and
future generations. Scope resolution enforces the namespace hierarchy: a rule
can target only its own namespace or descendants.

## Actions and projections

Imperative operations use TypeMeta, `metadata.target`, action input in `spec`,
and server results in `status`:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectorArchive
metadata:
  target:
    apiVersion: authproxy.net/v1alpha1
    kind: Connector
    id: cxr_01example
spec:
  timeoutSeconds: 600
status:
  taskId: encrypted-task-info
```

Actions are typed transports, not durable resources. Read-only operational
objects such as RequestEvent, Notification, Task, and WorkflowInstance are
typed projections. Analytical and third-party protocol payloads remain
unwrapped where a resource envelope would misrepresent them, including proxy
request/response bodies, metrics query results, session protocols, OAuth
redirects, and health checks.

Connection setup data sources and OAuth scopes are read-only lists:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: ConnectionScopeList
metadata: {}
items:
  - name: read
    requested: true
    granted: true
  - name: admin
    requested: true
    granted: false
```

For route-by-route schemas, use the generated Swagger documents linked from
the [API reference](/reference/api/).
