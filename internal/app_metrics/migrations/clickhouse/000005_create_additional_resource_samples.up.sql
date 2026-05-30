CREATE TABLE IF NOT EXISTS app_metrics_connector_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'connector',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    state String,
    connector_version UInt64 DEFAULT 0,
    total_versions Int64 DEFAULT 0,
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id);

CREATE TABLE IF NOT EXISTS app_metrics_connector_version_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'connector_version',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    state String,
    connector_version UInt64 DEFAULT 0,
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id, connector_version);

CREATE TABLE IF NOT EXISTS app_metrics_namespace_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'namespace',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    state String,
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id);

CREATE TABLE IF NOT EXISTS app_metrics_rate_limit_resource_samples (
    sampled_at_ms Int64,
    resource_type String DEFAULT 'rate_limit',
    resource_id String,
    namespace String,
    labels String DEFAULT '{}',
    mode String,
    resource_created_at_ms Int64,
    resource_updated_at_ms Int64,
    resource_deleted_at_ms Nullable(Int64),
    ingested_at_unix_nano Int64
) ENGINE = ReplacingMergeTree(ingested_at_unix_nano)
ORDER BY (sampled_at_ms, resource_id);
