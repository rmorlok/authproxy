-- Mirrors database.EnumerateOAuth2TokensExpiringWithin's first 100-row page.
EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT t.*, c.*
FROM oauth2_tokens AS t
INNER JOIN connections AS c ON c.id = t.connection_id
WHERE t.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND c.state = 'configured'
  AND t.access_token_expires_at <= CURRENT_TIMESTAMP + INTERVAL '5 minutes'
ORDER BY t.created_at DESC
LIMIT 101 OFFSET 0;
