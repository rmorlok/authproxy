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
    autoMigrate: true
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

See [Automatic migrations](/operations/migrations/) for startup locking and
lease-renewal behavior across SQLite, PostgreSQL, and ClickHouse.

## Request events

List request-event metadata with `GET /api/v1/metrics/request-events`. Filters
include namespace, connector, connection, method, status range, path, response
source, rate-limit id, label selector, and timestamp range. Fetch one event at
`GET /api/v1/metrics/request-events/{id}`.

Full request and response payloads are separate encrypted blobs and exist only
when `fullRequestRecording` is `always`. Keep recording at `never` unless the
debugging or audit requirement justifies the additional sensitive data,
storage, access control, and retention burden.

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
