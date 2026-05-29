ALTER TABLE app_metrics_request_events
    ADD COLUMN request_body_skipped TEXT NOT NULL DEFAULT '',
    ADD COLUMN response_body_skipped TEXT NOT NULL DEFAULT '';
