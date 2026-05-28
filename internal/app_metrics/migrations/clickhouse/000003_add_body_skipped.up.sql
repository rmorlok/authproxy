ALTER TABLE app_metrics_request_events
    ADD COLUMN IF NOT EXISTS request_body_skipped String DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_body_skipped String DEFAULT '';
