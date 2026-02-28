CREATE TABLE IF NOT EXISTS http_log_entry_records (
    request_id TEXT PRIMARY KEY,
    namespace TEXT NOT NULL,
    type TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    timestamp_ms BIGINT NOT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    connection_id TEXT NOT NULL DEFAULT '',
    connector_id TEXT NOT NULL DEFAULT '',
    connector_version BIGINT NOT NULL DEFAULT 0,
    method TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    scheme TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    response_status_code INTEGER NOT NULL DEFAULT 0,
    response_error TEXT NOT NULL DEFAULT '',
    request_http_version TEXT NOT NULL DEFAULT '',
    request_size_bytes BIGINT NOT NULL DEFAULT 0,
    request_mime_type TEXT NOT NULL DEFAULT '',
    response_http_version TEXT NOT NULL DEFAULT '',
    response_size_bytes BIGINT NOT NULL DEFAULT 0,
    response_mime_type TEXT NOT NULL DEFAULT '',
    internal_timeout BOOLEAN NOT NULL DEFAULT FALSE,
    request_cancelled BOOLEAN NOT NULL DEFAULT FALSE,
    full_request_recorded BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_entry_records_namespace ON http_log_entry_records (namespace);
CREATE INDEX IF NOT EXISTS idx_entry_records_timestamp ON http_log_entry_records (timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_entry_records_connection ON http_log_entry_records (connection_id);
CREATE INDEX IF NOT EXISTS idx_entry_records_connector ON http_log_entry_records (connector_id);
CREATE INDEX IF NOT EXISTS idx_entry_records_status ON http_log_entry_records (response_status_code);
