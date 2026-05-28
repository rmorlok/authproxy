ALTER TABLE app_metrics_request_events
    DROP COLUMN IF EXISTS request_body_skipped,
    DROP COLUMN IF EXISTS response_body_skipped;
