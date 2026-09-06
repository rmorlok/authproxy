---
title: Application Metrics
---

AuthProxy stores Admin UI-facing application metrics in the `appMetrics` store. This store includes request-event records, optional full request/response payloads, and periodic resource snapshots used for time-series dashboards.

## Configuration

`appMetrics` is required because request-event listing and metrics queries are routed through it. In development the store can use the same SQLite or Postgres database as the primary application database; appMetrics tracks its migrations in `app_metrics_schema_migrations` so it does not conflict with the primary `schema_migrations` table. Deployed environments should prefer ClickHouse or a dedicated Postgres database when request volume is high.

```yaml
appMetrics:
  resourceSnapshotInterval: 15m
  database:
    provider: clickhouse
    addresses:
      - localhost:8123
    database: authproxy
    user: authproxy
    password: authproxy
  requestEvents:
    fullRequestRecording: never
  blobStorage:
    provider: s3
    bucket: authproxy-request-logs
```

Key settings:

| Setting | Purpose |
|---|---|
| `appMetrics.database` | Database for request events and resource sample tables; can be shared with the primary application database for development. |
| `appMetrics.resourceSnapshotInterval` | Cadence for the worker snapshot job. Defaults to `15m`. |
| `appMetrics.requestEvents.fullRequestRecording` | `never` (default) or `always`; controls whether full request/response bodies are captured. |
| `appMetrics.blobStorage` | Stores full request/response payloads when capture is enabled. |

The resource snapshot worker stores live resources at each interval. Deleted resources remain visible in historical time slices where they were sampled, but they are excluded from later snapshots.

See [Database migrations](/operations/migrations/) for status inspection,
explicit migration commands, and locking behavior across SQLite, PostgreSQL,
and ClickHouse.

## Request events

List request-event metadata with `GET /api/v1/metrics/request-events`. Filters
include namespace, connector, connection, method, status range, path, response
source, rate-limit id, label selector, and timestamp range. Fetch one event at
`GET /api/v1/metrics/request-events/{id}`.

Request events use the `authproxy.net/v1alpha1` envelope, but they are immutable
observations rather than resources with client-managed desired state. Event
identity, namespace, the frozen label snapshot, and timestamp are in
`metadata`; observed request and response facts are in `spec`. Resource
attribution uses typed references. In particular, `connectorRef.generation`
records the connector generation used for the request.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: RequestEvent
metadata:
  id: req_01example
  namespace: root.acme
  labels:
    team: payments
  createdAt: 2026-09-06T17:30:00Z
spec:
  requestType: proxy
  correlationId: corr-123
  durationMilliseconds: 150
  namespaceRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Namespace
    id: root.acme
  actorRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Actor
    id: act_01example
    name: billing-service
    namespace: root.acme
  connectionRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    id: cxn_01example
    name: production
    namespace: root.acme
  connectorRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connector
    id: cxr_01example
    name: provider
    namespace: root.integrations
    generation: 3
  request:
    method: GET
    host: api.example.com
    scheme: https
    path: /v1/items
  response:
    statusCode: 200
    source: upstream
  captureAvailable: false
```

List responses use `kind: RequestEventList`; pagination is returned in
`metadata.continue`, and the exact match count (when available) is returned in
`metadata.total`. Existing request-event filters retain their current query
parameter names, including `connectorVersion` for generation-specific queries.

Full request and response payloads are separate encrypted blobs and exist only
when `fullRequestRecording` is `always`. Keep recording at `never` unless the
debugging or audit requirement justifies the additional sensitive data,
storage, access control, and retention burden.

When capture is present, `spec.capture` contains the original URL, headers, and
request/response bodies; body values use base64 on the wire. These fields are
not metadata and are redacted by default. AuthProxy returns unredacted capture
only when the authenticated actor has `secrets:replay`; otherwise the response
includes `X-AuthProxy-Data-Redacted: true` when capture fields were masked.
Treat replayed data as sensitive even when a particular request appears
harmless.

## Query API

Use `POST /api/v1/metrics/query` with a time range, optional namespace matcher, optional label selector, and one or more query refs.

```json
{
  "range": {
    "start": "2026-05-25T12:00:00Z",
    "end": "2026-05-25T13:00:00Z",
    "step": "15m"
  },
  "namespace": "root.**",
  "labelSelector": "env=prod",
  "queries": [
    {
      "refId": "connections",
      "metric": "resources.connections",
      "aggregation": "count",
      "groupBy": ["state", "health_state"]
    }
  ]
}
```

Responses are returned as labeled time series:

```json
{
  "series": [
    {
      "refId": "connections",
      "metric": "resources.connections",
      "aggregation": "count",
      "labels": {
        "state": "configured",
        "healthState": "healthy"
      },
      "points": [
        {"timestamp": "2026-05-25T12:00:00Z", "value": 4}
      ]
    }
  ]
}
```

## Metrics

Request-event metrics are computed from stored request events.

| Metric | Aggregations | `group_by` |
|---|---|---|
| `request_events` | `count` | `type`, `method`, `response_status_code`, `response_source`, `connector_id` |
| `request_events.errors` | `count` | `type`, `method`, `response_status_code`, `response_source`, `connector_id` |
| `request_events.duration_ms` | `avg`, `p95` | `type`, `method`, `response_status_code`, `response_source`, `connector_id` |

Resource metrics are computed from periodic app-metrics resource samples.

| Metric | Aggregations | `group_by` |
|---|---|---|
| `resources.connections` | `count` | `state`, `health_state`, `connector_id`, `connector_version` |
| `resources.actors` | `count` | `namespace` |
| `resources.connectors` | `count` | `state`, `connector_version`, `namespace` |
| `resources.connector_versions` | `count` | `state`, `connector_id`, `connector_version`, `namespace` |
| `resources.namespaces` | `count` | `state`, `namespace` |
| `resources.rate_limits` | `count` | `mode`, `namespace` |

All metric queries accept the same namespace matcher and label selector fields. Label selectors evaluate against the frozen labels stored with the request event or resource sample, not the current live resource.
Implicit `apxy/<rt>/-/name` labels therefore preserve the name captured in that
sample or request snapshot. Renaming a live resource does not rewrite
historical metrics data.
