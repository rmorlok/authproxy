---
title: Host application integration
---

The host application remains the source of truth for users, tenants, roles,
and product installations. AuthProxy should receive stable identifiers and the
minimum metadata needed to authorize and operate third-party connections.

## Map host entities deliberately

| Host entity | AuthProxy representation | Guidance |
|---|---|---|
| User or service principal | Actor `spec.externalId` | Use an immutable primary key, not email or display name. |
| Tenant or organization | Namespace | Derive a stable, namespace-safe segment and never rename it when the tenant's display name changes. |
| Team | Optional child namespace | Create it only when it is an authorization boundary. Otherwise use a label. |
| Integration installation | Connection | Store the immutable `cxn_...` ID in the host. Supply a human-readable name for display and exact-name discovery. |
| Searchable host metadata | Labels | Keep values short and non-sensitive; labels appear in request-event dimensions. |
| Descriptive metadata | Annotations | Use for non-selectable values such as a host record URL or description. |

An actor is located by `(namespace, externalId)`, so keep both stable. A user
with host id `usr_7` in tenant `tnt_42` might map to:

```text
actor namespace: root.tenants.tnt_42
externalId:     usr_7
tenant namespace: root.tenants.tnt_42
```

If the host id contains characters that cannot appear in a namespace segment,
retain the original id as `externalId` and maintain a deterministic,
namespace-safe key separately.

## Choose a namespace model

### Tenant-shared connections

```text
root.tenants.tnt_42
├── actors: usr_7, usr_9
└── connections: Salesforce, Slack
```

Give each tenant actor access to the tenant subtree. Use this when one provider
authorization represents the whole customer account.

### Per-user connections

```text
root.tenants.tnt_42
└── users
    ├── usr_7
    │   └── connection: Google Drive
    └── usr_9
        └── connection: Google Drive
```

Give an actor access only to its user subtree. Use this when the provider
authorization represents an individual account.

Many products use both models. A connector can live at a parent namespace while
each connection is created in the appropriate child namespace.

## Provision actors

AuthProxy supports two provisioning patterns:

1. **Provision first.** Create or synchronize the actor through a trusted
   management path. Browser handoff JWTs then identify it by `sub` and
   `namespace`. A token that does not contain a full actor claim is rejected if
   that actor does not already exist.
2. **Just in time.** A trusted JWT may contain the complete actor claim.
   AuthProxy upserts that actor during authentication. Use this only when the
   host signing service is authoritative for actor permissions and metadata.

Provision-first is easier to audit. Just-in-time provisioning reduces sync work
but makes the token issuer part of the actor-provisioning control plane.

An embedded actor uses the same `authproxy.net/v1alpha1` resource shape as the
Actor API, with a deliberately restricted field set:

```yaml
sub: usr_7
namespace: root.tenants.tnt_42
actor:
  apiVersion: authproxy.net/v1alpha1
  kind: Actor
  metadata:
    name: user-7
    namespace: root.tenants.tnt_42
    labels:
      app.example.com/role: member
  spec:
    externalId: usr_7
    permissions:
      - namespace: root.tenants.tnt_42.**
        resources: [connections]
        verbs: [create, get, list, proxy]
```

The JWT `sub` must equal `actor.spec.externalId`, and the top-level
`namespace` must equal `actor.metadata.namespace`. Do not include
`metadata.id`, `metadata.generation`, lifecycle timestamps, `status`, or
`spec.signingKey` in a JWT actor. Those fields are database identity,
server-observed state, or secret configuration—not identity assertions. The
legacy flat actor claim is not accepted.

Do not copy an entire host user profile. AuthProxy generally needs the stable
id, home namespace, permissions, and carefully selected labels or annotations.

## Define least-privilege access

For a tenant user who can browse connectors and use connections, keep connector
and connection permissions separate:

```json
[
  {
    "namespace": "root.tenants.tnt_42.**",
    "resources": ["connectors"],
    "verbs": ["list", "get"]
  },
  {
    "namespace": "root.tenants.tnt_42.**",
    "resources": ["connections"],
    "verbs": ["create", "list", "get", "update", "disconnect", "proxy"]
  }
]
```

Treat this as a starting example, not a universal Marketplace role. Remove
verbs the user experience does not need. Keep connector publication,
force-state actions, actor management, and namespace administration on a
separate operator identity. If connectors live in a shared parent namespace,
grant their `list` and `get` permissions there while keeping connection
permissions restricted to the tenant or user namespace.

Request JWTs may narrow these permissions. AuthProxy verifies that every
top-level token permission is a subset of the actor's base permissions and
then intersects the two sets during authorization. A broader token is rejected
instead of being silently trimmed.

## Track host installations

There are three common lookup patterns:

- **Store the connection id.** Add `cxn_...` to the host installation row. This
  is the most direct lookup for proxy requests.
- **Assign a resource name.** Use a stable, human-readable value such as
  `production-crm` in UI lists and reconciliation tools. Exact-name queries are
  namespace-scoped; names can be changed and must not replace the stored ID.
- **Apply a label.** Add a label such as
  `app.example.com/installation-id=ins_123` and query with a label selector.
  This is useful for reconciliation and audit searches.

Using all three gives fast direct access, readable operator output, and
recoverable reconciliation. Never use a mutable AuthProxy name or a
user-editable provider account name as the durable join key.

Labels improve discovery; namespace permissions still enforce access. See
[Labels and annotations](/concepts/labels-and-annotations/) for selector,
carry-forward, and request-snapshot behavior.

## Handle membership and deletion

When tenant membership changes, update actor permissions before issuing another
session or API token. On account deletion:

1. stop issuing tokens and remove the actor's access;
2. disconnect connections that were private to that actor;
3. preserve tenant-shared connections still used by other actors; and
4. retain or purge request-event data according to the host's data policy.

Because connections are namespace-owned rather than actor-owned, the host must
make the shared-versus-private decision explicit.

## Next steps

- [Implement Marketplace SSO](/integration/marketplace/)
- [Review the core resource model](/concepts/core-model/)
- [Make requests through a connection](/sdks/proxying/)
