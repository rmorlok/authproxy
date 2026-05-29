DROP INDEX IF EXISTS idx_app_metrics_request_events_rate_limit_id;
DROP INDEX IF EXISTS idx_app_metrics_request_events_response_source;

ALTER TABLE app_metrics_request_events
    DROP COLUMN IF EXISTS rate_limit_matched,
    DROP COLUMN IF EXISTS rate_limit_bucket,
    DROP COLUMN IF EXISTS rate_limit_mode,
    DROP COLUMN IF EXISTS rate_limit_id,
    DROP COLUMN IF EXISTS response_source;
