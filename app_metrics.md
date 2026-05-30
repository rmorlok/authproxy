# Application Metrics

AuthProxy stores Admin UI-facing application metrics in the `app_metrics` store. This store includes request-event records, optional full request/response payloads, and periodic resource snapshots used for time-series dashboards.

## Configuration

`app_metrics` is required because request-event listing and metrics queries are routed through it. In development the store can use SQLite, Postgres, or ClickHouse. Deployed environments should prefer ClickHouse or a dedicated Postgres database when request volume is high.

```yaml
app_metrics:
  resource_snapshot_interval: 15m
  database:
    provider: clickhouse
    auto_migrate: true
    addresses:
      - localhost:8123
    database: authproxy
    user: authproxy
    password: authproxy
  request_events:
    full_request_recording: errors
  blob_storage:
    provider: s3
    bucket: authproxy-request-logs
```

Key settings:

| Setting | Purpose |
|---|---|
| `app_metrics.database` | Dedicated database for request events and resource sample tables. |
| `app_metrics.resource_snapshot_interval` | Cadence for the worker snapshot job. Defaults to `15m`. |
| `app_metrics.request_events.full_request_recording` | Controls when full request/response bodies are captured. |
| `app_metrics.blob_storage` | Stores full request/response payloads when capture is enabled. |

The resource snapshot worker stores live resources at each interval. Deleted resources remain visible in historical time slices where they were sampled, but they are excluded from later snapshots.

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
  "label_selector": "env=prod",
  "queries": [
    {
      "ref_id": "connections",
      "metric": "resources.connections",
      "aggregation": "count",
      "group_by": ["state", "health_state"]
    }
  ]
}
```

Responses are returned as labeled time series:

```json
{
  "series": [
    {
      "ref_id": "connections",
      "metric": "resources.connections",
      "aggregation": "count",
      "labels": {
        "state": "configured",
        "health_state": "healthy"
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
