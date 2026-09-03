---
title: Rate Limits
---

AuthProxy has two complementary rate-limiting systems:

1. **Connector-level reactive rate limiting** — built into every connector. When a 3rd party returns 429, AuthProxy parses the `Retry-After` header and blocks subsequent requests on that connection until the wait expires. This protects you from hammering the upstream after it has already told you to slow down.
2. **Rate limit resources** — declarative, namespace-scoped rules you define via API or Terraform. They run **before** the upstream call and reject requests that would breach a quota you have configured (e.g., "no more than 60 writes per minute per actor against Salesforce").

Both can fire on the same request and both stamp the request log so you can tell them apart. This page covers each in detail.

## When to use which system

| Need | Use |
|---|---|
| Stop hammering a 3rd party that just returned 429. | The reactive limiter — built in, no configuration required for the common case. |
| Tune what counts as a "retryable" 429 and how aggressively to back off. | Configure `rateLimiting` on the connector definition. |
| Cap your own usage to stay below a known 3rd-party quota. | Define a rate-limit resource with `enforce` mode. |
| Cap per-actor / per-team / per-cohort usage so noisy neighbours don't drain a shared quota. | Define a rate-limit resource with bucket dimensions. |
| Roll out a new limit safely — see how many requests it *would have* rejected before turning it on. | Define a rate-limit resource with `observe` mode. |
| Both — back off when upstream says 429 **and** never exceed our own configured cap. | Use both. They coexist; both can fire on the same request. |

The rest of this page is mostly about the second system (rate-limit resources). The [connector-level reactive limiter](#connector-level-reactive-429-handling) gets its own section near the end.

## Defining a rate limit

A rate-limit resource lives in a namespace. It applies to proxy / probe traffic against connections in that namespace and any descendant namespace.

### Via the API

```http
POST /api/v1/rate-limits
Content-Type: application/json
Authorization: Bearer <admin token>

{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "RateLimit",
  "metadata": {
    "name": "salesforce-writes",
    "namespace": "root.acme",
    "labels": { "team": "acme" },
    "annotations": { "owner": "platform@example.com" }
  },
  "spec": {
    "mode": "enforce",
    "selector": {
      "labelSelector": "apxy/cxr/type=salesforce",
      "methods": ["POST", "PATCH", "PUT"],
      "pathMatch": {
        "kind": "prefix",
        "value": "/services/data/"
      }
    },
    "bucket": {
      "dimensions": ["actor", "labels/team"]
    },
    "algorithm": {
      "tokenBucket": { "capacity": 60, "refillRate": 1.0 }
    }
  }
}
```

The response uses the same envelope. `metadata.id` contains the server-assigned id (e.g., `rl_AbcXyz…`), `metadata` carries the materialised label set and timestamps, and `status.effectiveMode` reports the resolved mode (`enforce` when `spec.mode` is omitted). The implicit `apxy/rl/-/id` / `apxy/rl/-/ns` labels and any carried-forward namespace labels are described under [Labels](/concepts/labels-and-annotations/).

Update with `PATCH /api/v1/rate-limits/{id}`. Send the resource type plus `metadata` and `spec` patch objects; omit fields inside those objects when they should remain unchanged:

```json
{
  "apiVersion": "authproxy.net/v1alpha1",
  "kind": "RateLimit",
  "metadata": {},
  "spec": {"mode": "observe"}
}
```

Set `spec.scope` to `null` to restore namespace scope. Delete with `DELETE /api/v1/rate-limits/{id}`. List with `GET /api/v1/rate-limits` (supports `namespace`, `name`, `labelSelector`, and pagination cursor params).

Labels and annotations also have sub-resource endpoints — see [Labels — API surface](/concepts/labels-and-annotations/#api-surface).

### Via server configuration

The `rateLimits.loadFromList` configuration accepts the same canonical YAML resource. Startup reconciliation preserves an explicit `metadata.id`, or matches by `metadata.namespace` and `metadata.name` when the ID is omitted:

```yaml
rateLimits:
  loadFromList:
    - apiVersion: authproxy.net/v1alpha1
      kind: RateLimit
      metadata:
        name: salesforce-writes
        namespace: root.acme
        labels:
          team: acme
      spec:
        scope:
          connectorRef:
            apiVersion: authproxy.net/v1alpha1
            kind: Connector
            namespace: root.acme
            name: salesforce
            generation: 3
        mode: enforce
        selector:
          methods: [POST, PATCH, PUT]
        bucket:
          dimensions: [actor, labels/team]
        algorithm:
          tokenBucket:
            capacity: 60
            refillRate: 1.0
```

Connector references are resolved after configured connectors are reconciled. AuthProxy stores the stable connector ID and requested generation. The former flat rate-limit configuration is not accepted; update configuration before restarting on this breaking pre-production release.

## Resource scope

`metadata.namespace` always establishes the outer boundary: the rule can affect that namespace and its descendants, never a parent or sibling. An omitted `spec.scope` applies to `<metadata.namespace>.**`, including the owning namespace itself. Add exactly one namespace matcher or typed reference to narrow it:

```yaml
# Selected descendants of a centrally managed parent namespace
scope:
  namespaceMatcher: root.acme.payments.**
```

The matcher may be exact (`root.acme.payments`) or recursive (`root.acme.payments.**`). Its base namespace must be the RateLimit's `metadata.namespace` or a descendant. For example, a rule owned by `root.acme` cannot use `root.**` or `root.other.**`. This lets administrators keep a surgical rule in a parent namespace where child-namespace principals cannot remove it.

```yaml
# Every generation of one connector
scope:
  connectorRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connector
    id: cxr_01example
```

```yaml
# One connector generation
scope:
  connectorRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connector
    id: cxr_01example
    generation: 3
```

```yaml
# One connection
scope:
  connectionRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    id: cxn_01example
```

References may use `id`, or the pair `namespace` and `name`. Connector `generation` is optional and means all generations when omitted; it is not valid on a connection reference. AuthProxy resolves every reference and rejects it unless the target resource belongs to `metadata.namespace` or one of its descendants.

### Via Terraform

The `authproxy_rate_limit` resource exposes every field as typed HCL:

```hcl
resource "authproxy_rate_limit" "salesforce_writes" {
  namespace = authproxy_namespace.acme.path
  mode      = "enforce"

  labels = {
    team = "acme"
  }
  annotations = {
    owner = "platform@example.com"
  }

  scope {
    connector_ref {
      id         = authproxy_connector.salesforce.id
      generation = 3
    }
  }

  selector {
    label_selector = "apxy/cxr/type=salesforce"
    methods        = ["POST", "PATCH", "PUT"]
    path_match {
      kind  = "prefix"
      value = "/services/data/"
    }
  }

  bucket {
    dimensions = ["actor", "labels/team"]
  }

  algorithm {
    token_bucket {
      capacity    = 60
      refill_rate = 1.0
    }
  }
}
```

For namespace targeting, use `scope { namespace_matcher = "root.acme.payments.**" }`. The provider requires exactly one of `namespace_matcher`, `connector_ref`, or `connection_ref` whenever a scope block is present.

Plan-time validation catches "exactly one of `fixed_window` / `sliding_window` / `token_bucket`" before `terraform apply`. The `namespace` is `ForceNew` — changing it replaces the resource. See [`authproxy_rate_limit` reference](https://github.com/rmorlok/authproxy/blob/main/terraform/provider/docs/resources/authproxy_rate_limit.md) for the full attribute reference, and [`examples/`](https://github.com/rmorlok/authproxy/tree/main/terraform/provider/examples/resources/authproxy_rate_limit/) for three end-to-end examples (token bucket, observe-mode rollout, sliding-window counter).

## Selectors — picking which requests get limited

The `selector` block decides which requests the rule applies to. All clauses are ANDed:

| Clause | What it matches | Default if omitted |
|---|---|---|
| `labelSelector` | Per-request label snapshot via [K8s-style selector syntax](/concepts/labels-and-annotations/#label-selectors). | Match any. |
| `methods` | HTTP verb (exact, upper-case). | Match any. |
| `pathMatch` | Final upstream URL path (after connector templating / rewriting). Three flavours: `prefix`, `glob` (`*` doesn't cross `/`), `regex`. | Match any. |
| `requestTypes` | What kind of traffic — `proxy`, `probe`, `oauth2_token_exchange`, etc. | `[proxy, probe]`. |

### Examples

**By actor and tenant**, via the per-request label snapshot:
```json
"selector": {
  "labelSelector": "apxy/act/-/id=act_AbcXyz,app.example.com/tenant-id=tenant-42"
}
```

**By connector type** — pin the rule to all Salesforce connections in the namespace:
```json
"selector": {
  "labelSelector": "apxy/cxr/type=salesforce"
}
```

(`apxy/cxr/type` is carried forward from the connector version's user labels into each connection it's used by — see [Labels — Carry-forward](/concepts/labels-and-annotations/#carry-forward--how-labels-flow-through-the-hierarchy).)

**By path** — limit only writes against the Salesforce REST API:
```json
"selector": {
  "methods": ["POST", "PATCH", "PUT"],
  "pathMatch": { "kind": "prefix", "value": "/services/data/" }
}
```

**By specific operation** — regex when prefix / glob aren't precise enough:
```json
"selector": {
  "pathMatch": {
    "kind": "regex",
    "value": "^/v1/users/[0-9]+/permissions$"
  }
}
```

### Request types

Most rules govern user-driven `proxy` traffic and connector-defined `probe` traffic — that's the default. **OAuth2 token exchange / refresh / revocation are *not* governed by default** because rate-limiting your own auth flows is usually a self-inflicted outage. If you do need to throttle those (e.g., a 3rd party that throttles refresh aggressively), opt in explicitly:

```json
"selector": { "requestTypes": ["oauth2_refresh"] }
```

An explicit empty `requestTypes: []` is rejected at validation — omit the field to use the default.

## Buckets — projecting requests into independent counters

A bucket projects matched requests into separate counters so a limit can be "per actor" or "per team" or "per actor + team" rather than one global counter for the rule.

```json
"bucket": {
  "dimensions": ["actor", "labels/team"]
}
```

Each unique combination of values is its own counter. So `{actor=act_a, team=acme}` and `{actor=act_b, team=acme}` are two counters; both `act_a` calls to two different teams are also two counters.

| Reserved dimension | Resolves to |
|---|---|
| `actor` | The initiating actor's id (`apxy/act/-/id`). Empty for system / unauthenticated traffic. |
| `connection` | The connection's id. |
| `connector` | The connector's id. |
| `connector_version` | The numeric connector version. |
| `namespace` | The namespace path. |
| `method` | The HTTP verb. |
| `labels/<key>` | A per-request label value. Missing = empty string. |

An **empty `dimensions` list** is a single global counter for the rule — useful for whole-namespace or whole-system caps.

A missing-but-referenced dimension resolves to `""`. So a rule that buckets by `actor` and gets unauthenticated traffic puts all of that traffic in the `actor=""` bucket — distinct from any populated actor.

## Algorithms

Pick exactly one algorithm. Each makes a different tradeoff between accuracy, memory, and burst tolerance.

### `fixedWindow`

A counter that resets at boundaries derived from `floor(now / window)`.

```json
{ "fixedWindow": { "window": "1m", "limit": 100 } }
```

Simple and cheap. Susceptible to **boundary bursts** — a client can fire `limit` calls in the last 100 ms of one window and another `limit` in the first 100 ms of the next, getting `2 × limit` in a 200 ms interval.

### `slidingWindow` — `log` mode

A precise sliding window. Stores a timestamped log of recent allowed requests and removes entries older than `window` on each evaluation.

```json
{ "slidingWindow": { "window": "1m", "limit": 100, "mode": "log" } }
```

Exactly `limit` requests permitted in any rolling `window` interval. Most accurate; most memory (a sorted set per bucket).

### `slidingWindow` — `counter` mode

An approximation using two adjacent fixed-window counters, weighted by how far into the current window we are. Cheaper than `log` mode; accurate to a small constant factor.

```json
{ "slidingWindow": { "window": "1m", "limit": 100, "mode": "counter" } }
```

Use when you need sliding-window semantics without the per-request memory cost.

### `tokenBucket`

A bucket of tokens refilled at a constant rate. Each request consumes one token; if the bucket is empty, the request is rejected.

```json
{ "tokenBucket": { "capacity": 60, "refillRate": 1.0 } }
```

`capacity` is the burst — the max tokens the bucket can hold. `refillRate` is tokens per second (may be fractional, e.g. `0.5` = one new token every two seconds). New buckets start full so first-time callers get the configured burst capacity rather than instantly hitting an empty pool.

## `enforce` vs. `observe` mode

| Mode | Behaviour |
|---|---|
| `enforce` (default) | When the rule rejects, return a 429 to the caller. |
| `observe` | When the rule rejects, **pass the request through to upstream anyway** but record the would-have-rejected event on the request log. |

`observe` is the safe-rollout switch. Deploy a new rule in `observe`, watch the request log for a few days, confirm the match volume / bucket distribution look sensible, then flip to `enforce`.

Observe-mode rules **still increment counters** so when you flip to `enforce` the buckets are already warmed up — you don't see an instant spike of rejections from cold buckets.

## What the caller sees on rejection

When an `enforce`-mode rule rejects a proxy request, AuthProxy returns:

| Header / field | Value |
|---|---|
| HTTP status | `429 Too Many Requests` |
| `Retry-After` header | Seconds until the next request from this bucket would be permitted (window remainder for window algorithms; time-to-next-token for token bucket). |
| `X-Authproxy-Ratelimited` header | `true` — distinguishes any AuthProxy synthetic 429 from a real upstream 429. |
| `X-Authproxy-Ratelimit` header | The firing rule's id (e.g., `rl_AbcXyz…`). |
| Body | JSON: `{ "error": "rate limited", "rateLimitId": "<id>", "retryAfterSeconds": <n> }` |

Your application should treat 429 as a transient error and back off according to `Retry-After`. The `X-Authproxy-Ratelimit` header lets you correlate a 429 with the specific rule for support / debugging.

The connector-level reactive limiter ([below](#connector-level-reactive-429-handling)) also produces a 429 with `X-Authproxy-Ratelimited: true` but **no** `X-Authproxy-Ratelimit` header (it has no rule id). Use the request log's `responseSource` field (`upstream` / `connector_rate_limiter` / `rate_limit`) to disambiguate after the fact.

## Multiple matching rules

A request can match several rules at once — e.g., one rule at the root namespace, one at the child, and one targeting a specific label. AuthProxy evaluates **all matching rules** for every request and:

- **Most-restrictive wins.** If more than one `enforce`-mode rule rejects, the one with the longest `Retry-After` is the one whose id ends up in `X-Authproxy-Ratelimit` and in the response body. The caller sees a single 429 with the most pessimistic wait.
- **Observe rules never reject** but still evaluate (and increment their counters). Their decisions are recorded in the request log so you can see what would have happened.
- **The full match set lives on the log entry.** Every rule that matched — firing, observe, or didn't-reject — is recorded under `rateLimitMatched` on the request log entry. See [Request log attribution](#request-log-attribution).

Composition is "all apply" — there is no priority field, no specificity scoring, no first-match-wins. Layer org-wide caps at the root namespace with per-team caps at child namespaces and they stack cleanly.

## Namespace inheritance and permissions

A rate limit defined in namespace `N` applies to requests against connections in `N` **and any descendant namespace** by default. An explicit `namespaceMatcher`, `connectorRef`, or `connectionRef` narrows that inherited scope. Define a rule at `root` to cover the whole system, or keep it at `root` and use `namespaceMatcher: root.team-acme.**` when the rule must be managed centrally but enforced only for that team's traffic.

The implicit `apxy/rl/-/ns` label records the rule's home namespace, and the rule's user labels are inherited from that namespace via [carry-forward](/concepts/labels-and-annotations/#carry-forward--how-labels-flow-through-the-hierarchy).

Permission to create / read / update / delete a rate limit follows the standard AuthProxy permission model — a token can manage rate limits in any namespace in which it has `rate_limits` access.

## Connector-level reactive 429 handling

The reactive limiter is built into every connector. When a 3rd party returns 429, AuthProxy parses the response, blocks the connection for the suggested wait, and short-circuits subsequent requests on that connection with its own 429 until the cool-down expires. This prevents you from hammering an API that has already told you to slow down.

Configuration lives on the connector definition under `rateLimiting`. Every field has a sensible default — you only need to set the ones you want to override.

```yaml
# In a connector definition YAML
rateLimiting:
  disabled: false                       # Set true to pass 429s through unchanged
  retryAfterHeaders: ["Retry-After"]  # Headers to check, in priority order
  maxRetryAfter: 15m                  # Cap on any wait
  defaultRetryAfter: 60s              # Fallback when no parseable header found
  exponentialBackoff:                  # Used on consecutive 429s without a header
    initialInterval: 1s
    multiplier: 2.0
    maxInterval: 5m
    jitterFraction: 0.1
```

| Field | Default | What it does |
|---|---|---|
| `disabled` | `false` | When `true`, 429s pass through to the caller unchanged; no cool-down is recorded. |
| `retry_after_headers` | `["Retry-After"]` | Headers to inspect, in order. First parseable value wins. Supports integer seconds, RFC 7231 HTTP-date, and ISO 8601 timestamps. |
| `max_retry_after` | `15m` | Hard cap on any cool-down — protects against unreasonably long retry-after values from misbehaving upstreams. |
| `default_retry_after` | `60s` | Used when a 429 has no parseable header and exponential backoff is not configured. |
| `exponential_backoff.initial_interval` | `1s` | First-429 backoff. |
| `exponential_backoff.multiplier` | `2.0` | Applied per consecutive 429. |
| `exponential_backoff.max_interval` | `5m` | Cap on the per-step backoff. |
| `exponential_backoff.jitter_fraction` | `0.1` | Each computed backoff is uniformly sampled in `[(1−j)·t, (1+j)·t]`. Prevents thundering-herd retries. |

When the reactive limiter short-circuits a request, the synthetic 429 carries `X-Authproxy-Ratelimited: true` and shows up in the request log with `responseSource: connector_rate_limiter`. No `X-Authproxy-Ratelimit` header (there's no rule id — this is opportunistic, not a configured rule).

## Request log attribution

Every 429 — real upstream, connector-level reactive, or rate-limit resource — is recorded on the request log with a `responseSource` field so you can tell them apart:

| `responseSource` | Means |
|---|---|
| `upstream` (default) | The response (incl. any 429) came from the 3rd party. |
| `connector_rate_limiter` | The connector-level reactive limiter short-circuited the request because the connection was in cool-down. No upstream call was made. |
| `rate_limit` | A rate-limit resource rejected the request before any upstream call. |

When a rate-limit resource is involved (firing or in observe-mode), the request log entry also carries:

| Field | When populated | Notes |
|---|---|---|
| `rateLimitId` | Any time a rate-limit rule matched (incl. observe-only). | The most-restrictive firing rule, or the first observe match if none fired. |
| `rateLimitMode` | Same. | `enforce` or `observe`. |
| `rateLimitBucket` | Same. | The resolved dimension → value map for the matched rule. |
| `rateLimitMatched` | Same. | Full list of every rule that matched: `[{id, mode, bucket}, …]` — observe rules included. |

The list endpoint accepts `responseSource` and `rateLimitId` filters so you can scope a request-log search to "every request rejected by `rl_AbcXyz`" or "every 429 that came from upstream".

```http
GET /api/v1/metrics/request-events?responseSource=rate_limit&rateLimitId=rl_AbcXyz
```

The admin UI surfaces `responseSource` as a column (chip-rendered with a colour for synthetic-429 sources) and `rateLimitId` as a hidden-by-default column with a link to the rate-limit detail page.

## Implementation reference

The rest of this document covers internals — useful if you're operating or debugging an AuthProxy deployment, or if you're reading the request-log fields and want to know exactly what they mean.

### Architecture

The proxy-side rate-limit feature is composed of four moving parts:

```
            ┌────────────────────────────────────────────┐
            │   admin API: CRUD rate_limit resources     │
            └───────────────┬────────────────────────────┘
                            │
                            ▼
            ┌────────────────────────────────────────────┐
            │   Postgres: rate_limits table              │
            └───────────────┬────────────────────────────┘
                            │ refreshed every 5 min
                            ▼
            ┌────────────────────────────────────────────┐
            │   In-memory rule cache (per proxy process) │◀─────┐
            └───────────────┬────────────────────────────┘       │
                            │                                     │
                            ▼                                     │
   [proxy request] ──▶  Match(rule, request) ──▶ Limiter.Decide(ctx, bucket_key)
                            │                          │
                            │                          ▼
                            │              ┌─────────────────────┐
                            │              │  Redis: counters    │
                            │              └─────────────────────┘
                            │
                            ▼
                  synth 429   /   pass through
                  +  stamp request_log.Attribution
```

- **Rule cache** — every proxy process holds an in-memory snapshot of all rate-limit rules. A background goroutine refreshes from the database every 5 minutes (configurable, minimum 5 s). On Postgres failure the cache keeps its last-known-good snapshot.
- **Matcher** — pure function `Match(rule, request) → (matched, bucket_key)`. Evaluates the rule's selector clauses against the request's type / method / upstream URL / label snapshot.
- **Limiter** — per-algorithm Redis-backed counter. Each `Decide(ctx, bucket_key)` call runs a single Lua script so check-and-increment is atomic across processes.
- **Enforcer round-tripper** — runs in the HTTP client chain. Iterates the cache, calls `Match` then `Decide` on every match, picks the most-restrictive enforce rejection (or passes through), and stamps the request log Attribution either way.

The middleware chain on every proxied request is:

```
client → request log → telemetry → enforcer → connector reactive 429 → upstream
```

Request log is outermost so every request — including ones synth-rejected by either rate limiter — gets logged. Telemetry wraps both rate limiters so the client span covers any rate-limit waits. The enforcer runs before the reactive 429 limiter so a proxy-side rule rejection short-circuits cool-down checks. A real upstream 429 flows back through all of them.

### Bucket key resolution

The `BucketKey` for a matched request is the tuple of `(name, value)` pairs corresponding to the rule's `dimensions`, in the same order. Its canonical string form is used as the Redis sub-key:

```
actor=act_AbcXyz|labels/team=alpha
```

Pipe `|` separates components; `=` separates name from value. Both — plus `%` — are percent-encoded in values to keep the form unambiguous. An empty dimensions list yields the global key `*`. A missing-but-referenced dimension renders as the empty string (`actor=|labels/team=alpha`) — a distinct counter from any populated value.

### Algorithm internals

All three algorithms run their check-and-increment in a single Redis Lua script keyed on `ratelimit:rule:<rule_id>:<bucket_key>:<algo>:…`. Lua execution is atomic per shard, so even with thousands of concurrent goroutines hitting the same bucket the count is consistent.

| Algorithm | State per bucket | Atomic operation |
|---|---|---|
| `fixed_window` | One `INCR` counter at `…:fw:<window_id>` with TTL = window. Old windows self-expire. | `INCR` + `PEXPIRE`; reject if `> limit`, retry-after = `PTTL`. |
| `sliding_window` log | One ZSET at `…:swl` with `score=now_ms`, `member=<random tag>`. | `ZREMRANGEBYSCORE` (evict old) → `ZCARD` → reject + read oldest score for retry-after, or `ZADD` to admit. |
| `sliding_window` counter | Two `INCR` counters at `…:swc:<window_id>` and `:<window_id-1>` (current + previous). | Weighted average: `curr + floor(prev × (window − elapsed_in_curr) / window)`. |
| `token_bucket` | Hash at `…:tb` with `tokens`, `last_refill_ms`. New buckets start full. | Refill = `elapsed_s × rate`, cap at `capacity`; reject if `< 1` token, retry-after = `ceil((1 − tokens) / rate × 1000) ms`. |

Refill rates may be fractional (e.g. `0.5`) — the Lua arithmetic is float.

### Fail-open semantics

Every Redis call inside a `Limiter.Decide` is wrapped: on any error, the limiter returns `{Allowed: true, FailedOpen: true}` and logs a structured warning. Rate limits are guardrails, not security boundaries — a Redis blip should not produce a customer-visible outage.

Operationally:

- A persistent Redis outage means rate limits are effectively disabled while the outage lasts.
- The `FailedOpen` flag on the decision lets the enforcer surface this to operators via the structured log. Wire a metric on these events; sustained values indicate Redis health, not "you're under-utilising your limits".
- The matcher itself never fails open — it's a pure function, no I/O. Bad rule data (e.g., uncompilable regex that somehow escaped schema validation) is logged and the rule is skipped for that request.

### Coexistence with the reactive limiter

Both systems can fire on the same request. The order is determined by the middleware chain ([above](#architecture)):

1. **Enforcer first.** If a proxy-side rule rejects, you get a 429 with `responseSource: rate_limit` — the request never reaches the reactive limiter.
2. **Reactive limiter second.** If the enforcer passed through but the connection is in cool-down from a prior real upstream 429, you get a 429 with `responseSource: connector_rate_limiter`.
3. **Upstream third.** If both passed through, the upstream call happens. A real upstream 429 returns with `responseSource: upstream` and the reactive limiter records the cool-down for next time.

This ordering means proxy-side rule rejections "win" over connector cool-down rejections. From an attribution standpoint that's the right call — operators configured the rule intentionally, while cool-down is opportunistic.

### Performance characteristics

- **Cache reads** are O(1) lock-free (atomic snapshot pointer).
- **Match evaluation** is linear in the number of cached rules per request. For most deployments this is in the tens to low hundreds; tests pin the matcher to microsecond-scale per call.
- **Redis round-trips** are one Lua script invocation per matched rule. With a typical rule count and a sane bucket distribution, 1–3 round trips per proxied request.
- **No connection draining at refresh time.** The cache refresh swaps an atomic snapshot pointer; in-flight `Match` calls keep their old snapshot.

### Known limitations

- **Cross-process invalidation on admin-API writes is not yet wired.** When an admin creates or updates a rule, each proxy process picks it up on its next 5-minute cache refresh. For most rollouts this is fine — you typically deploy a rule, then wait a refresh interval before relying on it. If you need faster propagation, reduce the interval (down to the 5 s minimum) or restart proxy processes after a write.
- **Counter sharding is per Redis instance.** AuthProxy assumes a single logical Redis. Sharding across Redis clusters is not yet supported.
- **Bucket dimensions reference only request-time data.** They cannot reference data only available after the upstream call (e.g., response size, response status). That model would require a "post-call" evaluation pass which is not part of this design.
- **Observe-mode rules count toward Redis storage.** Even though they don't reject, they increment counters — by design, so flipping to enforce doesn't reset buckets. Be aware that observe rules consume the same Redis memory as enforce rules.
