-- Mirrors database:cleanup_stale_connections's ListConnectionsBuilder page.
EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT *
FROM connections
WHERE deleted_at IS NULL
  AND state = 'setup'
  AND setup_step_id IS NOT NULL
  AND updated_at < CURRENT_TIMESTAMP - INTERVAL '1 second'
ORDER BY created_at ASC, id ASC
LIMIT 101 OFFSET 0;
