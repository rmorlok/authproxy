ALTER TABLE http_log_entry_records
    ADD COLUMN IF NOT EXISTS response_source String DEFAULT 'upstream',
    ADD COLUMN IF NOT EXISTS rate_limit_id String DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_limit_mode String DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_limit_bucket String DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS rate_limit_matched String DEFAULT '[]';
