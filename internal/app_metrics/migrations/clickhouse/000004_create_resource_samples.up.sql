CREATE TABLE IF NOT EXISTS app_metrics_connection_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'connection',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    state String,
    health_state String,
    connector_id String,
    connector_version UInt64 DEFAULT 0,
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id);

CREATE TABLE IF NOT EXISTS app_metrics_actor_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'actor',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    external_id String DEFAULT '',
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id);
