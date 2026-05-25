ALTER TABLE http_log_entry_records
    ADD COLUMN IF NOT EXISTS request_body_skipped String DEFAULT '',
    ADD COLUMN IF NOT EXISTS response_body_skipped String DEFAULT '';
