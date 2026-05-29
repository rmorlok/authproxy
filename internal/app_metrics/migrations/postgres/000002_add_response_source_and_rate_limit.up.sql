ALTER TABLE app_metrics_request_events
    ADD COLUMN response_source TEXT NOT NULL DEFAULT 'upstream',
    ADD COLUMN rate_limit_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN rate_limit_mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN rate_limit_bucket JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN rate_limit_matched JSONB NOT NULL DEFAULT '[]';

CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_response_source
    ON app_metrics_request_events (response_source);

CREATE INDEX IF NOT EXISTS idx_app_metrics_request_events_rate_limit_id
    ON app_metrics_request_events (rate_limit_id)
    WHERE rate_limit_id <> '';
