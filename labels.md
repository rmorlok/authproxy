# Labels and Annotations

AuthProxy attaches Kubernetes-style **labels** (key=value metadata) and **annotations** (free-form key=value pairs) to every long-lived resource. Labels are the lightweight integration surface between AuthProxy and your host application: tag a connection with your tenant id, search for it later by selector, and have the same tag appear automatically on every request log entry that connection produces.

This page is the reference for the label system — what labels exist, how they propagate, and how to query them.

## Table of contents

- [Quick start](#quick-start)
- [Where labels and annotations live](#where-labels-and-annotations-live)
- [Label vs. annotation](#label-vs-annotation)
- [System labels — the `apxy/` namespace](#system-labels--the-apxy-namespace)
- [Carry-forward — how labels flow through the hierarchy](#carry-forward--how-labels-flow-through-the-hierarchy)
- [Per-request label snapshot](#per-request-label-snapshot)
- [Label selectors](#label-selectors)
- [API surface](#api-surface)
- [Implementation notes](#implementation-notes)

## Quick start

Tag a connection with your tenant id and environment:

```http
PATCH /api/v1/connections/cxn_abc123
Content-Type: application/json

{
  "labels": {
    "app.example.com/tenant-id": "tenant-42",
    "app.example.com/env": "production"
  }
}
```

List all connections in production for that tenant:

```http
GET /api/v1/connections?label_selector=app.example.com/tenant-id=tenant-42,app.example.com/env=production
```

Find every request log entry produced by that tenant:

```http
GET /api/v1/request-log?label_selector=app.example.com/tenant-id=tenant-42
```

You did not have to write a separate "AuthProxy connection id → my tenant id" mapping table. The label travelled with the connection onto every request log entry automatically (see [Carry-forward](#carry-forward--how-labels-flow-through-the-hierarchy)).

## Where labels and annotations live

Every namespace-scoped resource carries `labels` and `annotations` columns:

| Resource | Notes |
|---|---|
| `namespace` | Top of the hierarchy. Its labels carry forward to every resource defined in it. |
| `actor` | Users / service accounts. Labels carry forward to requests an actor initiates. |
| `connector` (versioned) | Each version is a separate resource with its own labels. |
| `connection` | Inherits labels from its connector version, plus the namespace. |
| `encryption_key` | Inherits from its namespace. |
| `rate_limit` | Inherits from its namespace. See [Rate limits](rate-limits.md). |
| `request_log` entry | The frozen label snapshot of the request that produced it. |

Request log entries store the per-request label snapshot (the values that were active when the request ran), not a live reference — so a label change on the connection does not retroactively change old log entries.

## Label vs. annotation

Both are `map[string]string`. The differences are operational:

| | Labels | Annotations |
|---|---|---|
| Purpose | Identification, selection | Arbitrary metadata, descriptions |
| Selectable? | Yes — label selectors filter list queries | No |
| Key format | DNS-subdomain prefix + name (max 253 + 63 chars) | Same prefix rules but values are unrestricted |
| Value format | Up to 63 chars, alphanumeric + `-_.` | Anything — up to a 256 KB total cap per resource |
| Carry forward? | Yes (see below) | No |

Use labels for things you'll search by. Use annotations for things you want to attach but never filter on (long descriptions, ticket links, ownership emails, etc.).

## System labels — the `apxy/` namespace

The `apxy/` prefix is reserved. User-written labels under that prefix are rejected at validation; in exchange, AuthProxy auto-populates a small set of system labels on every resource so consumers can locate, join, and filter by structural identity.

### Identifier labels

Every resource gets two implicit labels stamped on create / update:

| Label key | Value |
|---|---|
| `apxy/<rt>/-/id` | This resource's id |
| `apxy/<rt>/-/ns` | This resource's namespace path |

The `<rt>` token comes from the resource's apid prefix (strip the trailing `_`):

| Resource | `<rt>` |
|---|---|
| Namespace | `ns` |
| Actor | `act` |
| Connection | `cxn` |
| Connector version | `cxr` |
| Encryption key | `ek` |
| Rate limit | `rl` |

For example, a connection with id `cxn_abc123` in namespace `root.acme` always carries:

```
apxy/cxn/-/id: cxn_abc123
apxy/cxn/-/ns: root.acme
```

These are useful for **selecting the resource by its own id** (e.g., from a per-request snapshot when you don't have the connection id directly to hand) and for joining audit data across resources.

### Carry-forward labels

When a parent resource's labels appear on a child, they are **re-keyed** under the parent's resource-type token so they don't collide with the child's own keys. This is the central abstraction of label propagation; see the next section.

## Carry-forward — how labels flow through the hierarchy

The hierarchy looks like this:

```
namespace
   ├── actor                            (labels carry forward through "act/")
   ├── encryption_key                   (labels carry forward through "ns/")
   ├── connector (version)              (labels carry forward through "cxr/")
   │      └── connection                (labels carry forward through "cxn/")
   ├── rate_limit                       (labels carry forward through "ns/")
   └── (child namespace)                (labels carry forward through "ns/")
```

### The rule

When a child resource is created, every parent's user labels are copied onto the child, **re-keyed** under `apxy/<parent_rt>/<original_key>`:

- Parent namespace `root.acme` has label `team=alpha`
- Connection created in `root.acme` gets a materialised label `apxy/ns/team=alpha`
- A request through that connection gets the same `apxy/ns/team=alpha` recorded in its log entry

Anything already on the parent under `apxy/...` is **forwarded as-is** (no double-prefixing). So if `root.acme` itself inherited `apxy/ns/team=alpha` from `root` (which had `team=alpha`), the connection still ends up with `apxy/ns/team=alpha` — not `apxy/ns/apxy/ns/team`.

### Worked example

```
root                    labels: { env: prod }
  └─ root.acme          labels: { team: alpha }
        └─ connection   labels: { app.example.com/cohort: beta }
```

The connection's full label set after creation:

```
app.example.com/cohort: beta             # user-set
apxy/ns/team:           alpha            # carried from parent namespace
apxy/ns/env:            prod             # carried from root (transitively)
apxy/cxn/-/id:          cxn_…            # implicit identifier
apxy/cxn/-/ns:          root.acme        # implicit identifier
```

### Propagation on parent change

Carry-forward is **materialised on create** — and **eventually consistent** afterwards. If the parent's labels change later, the values on existing children are stale until the propagation job catches up; depending on fleet size, fan-out depth, and how rate-limited the reconciler is, that delay can be anywhere from a few seconds to several hours.

There are two propagation paths:

1. **Targeted refresh.** Admin API mutators (e.g., updating a namespace's labels) enqueue a background task that re-derives every descendant's `apxy/` mirror — running each row's update in its own short transaction so concurrent reads aren't blocked. Typical latency: seconds to a few minutes, longer for deep fan-outs.
2. **Daily consistency checker.** A scheduled job walks every resource type, compares the on-disk labels to what the carry-forward rule would compute, and corrects any drift. Belt-and-braces for the targeted refresh; the worst-case bound is "next daily run plus the time the walk itself takes" — minutes to hours.

**Operational implications:**

- Treat carry-forward labels as eventually-consistent reads. Application logic that needs to observe a fresh parent-label change should not rely on a child's mirrored `apxy/<parent>/...` having caught up yet.
- A label change on a deep namespace can fan out to many descendants. The propagation job batches updates and runs out of band; do not rely on a label change being visible to in-flight proxy requests within the next few seconds.
- Per-request label snapshots on **new** requests after the update reflect the parent change as soon as each individual descendant row is rewritten — so propagation progresses visibly through the fleet rather than landing atomically.

## Per-request label snapshot

When an application sends a request through the proxy, AuthProxy assembles a **label snapshot** for that request. This is what gets recorded on the request log entry and what label selectors evaluate against (e.g., the `label_selector` clause on a [rate limit](rate-limits.md)).

The snapshot is the union of:

1. **The connection's labels** (which already include namespace + connector carry-forward — see above).
2. **The actor's contribution**, if the request was initiated by an authenticated actor. The actor's user labels are re-keyed under `apxy/act/<key>`, plus the actor's own identifier labels (`apxy/act/-/id`, `apxy/act/-/ns`). Other `apxy/*` entries on the actor (e.g. its own namespace's `apxy/ns/*`) are **not** forwarded — those describe the actor's home context, not the request's.
3. **Per-request labels** supplied by the caller in the `labels` field of a proxy request. These are user labels only; `apxy/`-prefixed keys are rejected so callers can't impersonate system labels.

The composed snapshot is then stamped onto the request log entry's `labels` field, frozen at that point in time.

## Label selectors

Anywhere the API takes a `label_selector` query parameter, the syntax is Kubernetes-style:

| Form | Matches |
|---|---|
| `key=value`, `key==value` | key present and equal to value |
| `key!=value` | key absent **or** present with a different value |
| `key` | key present (any value) |
| `!key` | key absent |

Combine with commas — clauses are ANDed:

```
app.example.com/tenant-id=tenant-42,app.example.com/env=production,!debug
```

Selectors can target system labels too:

```
# Every connection in any descendant of root.acme:
apxy/ns/-/id=root.acme

# Every request log entry from connections of type "salesforce":
apxy/cxr/type=salesforce
```

## API surface

Every resource that supports labels exposes the same four sub-resource endpoints, layered on top of the resource's CRUD URLs. Substitute the resource's URL segment (e.g., `connections`, `rate-limits`, `keys`):

| Method + Path | Behaviour |
|---|---|
| `GET /api/v1/<resource>/:id/labels` | Read all labels |
| `GET /api/v1/<resource>/:id/labels/:label` | Read one label by key |
| `PUT /api/v1/<resource>/:id/labels/:label` | Set / update one label |
| `DELETE /api/v1/<resource>/:id/labels/:label` | Remove one label |

Annotations have the same shape under `/annotations`.

The resource-level `PATCH` endpoint additionally supports replacing the entire user-label set in one shot via a top-level `labels` field. `PUT`s on the sub-resource path use merge semantics; `PATCH` on the parent with `labels` populated is a full replace.

System (`apxy/`) labels survive a full replace — only user labels are replaced.

## Implementation notes

- **Validation.** Labels are validated on every write: keys follow `[<prefix>/]<name>` where the prefix is a DNS subdomain (max 253 chars) and the name is 1–63 chars from `[a-zA-Z0-9_.-]`, with values up to 63 chars. The `apxy/` prefix is reserved on user input.
- **Storage.** Labels are stored as a JSON column (`jsonb` on Postgres, `text` on SQLite, `String` on Clickhouse for the request log). Label selectors compile to provider-specific JSON predicates.
- **Annotations** share the same encoding but cap at 256 KB total per resource; values are not further validated.
- **Propagation transactions.** Each carry-forward update runs in its own short transaction so long-fan-out updates do not hold long-running locks. The daily consistency checker uses a rate-limited enumerator to bound resource use even on large fleets.
- **What's not carried forward.** Annotations don't propagate. Per-request labels don't end up on the connection. Once a request log entry is written, its label snapshot is frozen — future label changes on the connection don't backfill old log entries.
