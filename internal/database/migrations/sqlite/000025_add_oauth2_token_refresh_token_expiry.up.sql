-- refresh_token_expires_at: when the provider said the refresh token stops
-- working (refresh_token_expires_in and related response fields); null when
-- unknown. refresh_token_refresh_at: when the background task should
-- proactively refresh to keep the connection alive; null when a refresh
-- cannot extend the refresh token or background refresh is disabled.
alter table oauth2_tokens add column refresh_token_expires_at datetime;
alter table oauth2_tokens add column refresh_token_refresh_at datetime;
create index idx_oauth2_tokens_refresh_token_refresh_at
    on oauth2_tokens (refresh_token_refresh_at);
