CREATE TABLE IF NOT EXISTS app_metrics_request_events (
    request_id TEXT PRIMARY KEY,
    namespace TEXT NOT NULL,
    type TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    timestamp_ms INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    connection_id TEXT NOT NULL DEFAULT '',
    connector_id TEXT NOT NULL DEFAULT '',
    connector_version INTEGER NOT NULL DEFAULT 0,
    method TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    scheme TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    response_status_code INTEGER NOT NULL DEFAULT 0,
    response_error TEXT NOT NULL DEFAULT '',
    request_http_version TEXT NOT NULL DEFAULT '',
    request_size_bytes INTEGER NOT NULL DEFAULT 0,
    request_mime_type TEXT NOT NULL DEFAULT '',
    response_http_version TEXT NOT NULL DEFAULT '',
    response_size_bytes INTEGER NOT NULL DEFAULT 0,
    response_mime_type TEXT NOT NULL DEFAULT '',
    internal_timeout BOOLEAN NOT NULL DEFAULT FALSE,
    request_cancelled BOOLEAN NOT NULL DEFAULT FALSE,
    full_request_recorded BOOLEAN NOT NULL DEFAULT FALSE,
    labels TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_namespace ON app_metrics_request_events (namespace);
CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_timestamp ON app_metrics_request_events (timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_connection ON app_metrics_request_events (connection_id);
CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_connector ON app_metrics_request_events (connector_id);
CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_status ON app_metrics_request_events (response_status_code);
