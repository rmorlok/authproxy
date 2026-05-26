DROP INDEX IF EXISTS idx_entry_records_rate_limit_id;
DROP INDEX IF EXISTS idx_entry_records_response_source;

ALTER TABLE http_log_entry_records DROP COLUMN rate_limit_matched;
ALTER TABLE http_log_entry_records DROP COLUMN rate_limit_bucket;
ALTER TABLE http_log_entry_records DROP COLUMN rate_limit_mode;
ALTER TABLE http_log_entry_records DROP COLUMN rate_limit_id;
ALTER TABLE http_log_entry_records DROP COLUMN response_source;
