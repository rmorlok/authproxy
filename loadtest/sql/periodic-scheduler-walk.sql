-- Mirrors core.GetCronTasks's ListConnectionsBuilder page for setup/configured connections.
EXPLAIN (ANALYZE, BUFFERS, VERBOSE)
SELECT *
FROM connections
WHERE deleted_at IS NULL
  AND state IN ('setup', 'configured')
ORDER BY created_at ASC, id ASC
LIMIT 101 OFFSET 0;
