ALTER TABLE http_log_entry_records
    ADD COLUMN response_source TEXT NOT NULL DEFAULT 'upstream',
    ADD COLUMN rate_limit_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN rate_limit_mode TEXT NOT NULL DEFAULT '',
    ADD COLUMN rate_limit_bucket JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN rate_limit_matched JSONB NOT NULL DEFAULT '[]';

CREATE INDEX IF NOT EXISTS idx_entry_records_response_source
    ON http_log_entry_records (response_source);

CREATE INDEX IF NOT EXISTS idx_entry_records_rate_limit_id
    ON http_log_entry_records (rate_limit_id)
    WHERE rate_limit_id <> '';
