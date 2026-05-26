ALTER TABLE http_log_entry_records
    DROP COLUMN IF EXISTS rate_limit_matched,
    DROP COLUMN IF EXISTS rate_limit_bucket,
    DROP COLUMN IF EXISTS rate_limit_mode,
    DROP COLUMN IF EXISTS rate_limit_id,
    DROP COLUMN IF EXISTS response_source;
