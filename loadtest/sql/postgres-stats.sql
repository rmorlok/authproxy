SELECT now() AS captured_at,
       datname,
       numbackends,
       xact_commit,
       xact_rollback,
       blks_read,
       blks_hit,
       tup_inserted,
       tup_updated,
       tup_deleted,
       deadlocks
FROM pg_stat_database
WHERE datname = current_database();

SELECT mode, count(*) AS lock_count
FROM pg_locks
GROUP BY mode
ORDER BY mode;

SELECT pg_size_pretty(pg_database_size(current_database())) AS database_size;
