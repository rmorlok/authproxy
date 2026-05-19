ALTER TABLE http_log_entry_records
    ADD COLUMN request_body_skipped TEXT NOT NULL DEFAULT '',
    ADD COLUMN response_body_skipped TEXT NOT NULL DEFAULT '';
