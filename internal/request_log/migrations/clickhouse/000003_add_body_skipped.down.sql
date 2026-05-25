ALTER TABLE http_log_entry_records
    DROP COLUMN IF EXISTS request_body_skipped,
    DROP COLUMN IF EXISTS response_body_skipped;
