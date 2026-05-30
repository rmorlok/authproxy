CREATE TABLE IF NOT EXISTS app_metrics_connector_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'connector',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL,
    connector_version INTEGER NOT NULL DEFAULT 0,
    total_versions INTEGER NOT NULL DEFAULT 0,
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_connector_resource_samples_namespace ON app_metrics_connector_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_connector_resource_samples_sampled_at ON app_metrics_connector_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_connector_resource_samples_state ON app_metrics_connector_resource_samples (state);

CREATE TABLE IF NOT EXISTS app_metrics_connector_version_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'connector_version',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL,
    connector_version INTEGER NOT NULL DEFAULT 0,
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id, connector_version)
);

CREATE INDEX IF NOT EXISTS idx_connector_version_resource_samples_namespace ON app_metrics_connector_version_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_connector_version_resource_samples_sampled_at ON app_metrics_connector_version_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_connector_version_resource_samples_state ON app_metrics_connector_version_resource_samples (state);

CREATE TABLE IF NOT EXISTS app_metrics_namespace_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'namespace',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL,
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_namespace_resource_samples_namespace ON app_metrics_namespace_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_namespace_resource_samples_sampled_at ON app_metrics_namespace_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_namespace_resource_samples_state ON app_metrics_namespace_resource_samples (state);

CREATE TABLE IF NOT EXISTS app_metrics_rate_limit_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'rate_limit',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    mode TEXT NOT NULL,
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_rate_limit_resource_samples_namespace ON app_metrics_rate_limit_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_rate_limit_resource_samples_sampled_at ON app_metrics_rate_limit_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_rate_limit_resource_samples_mode ON app_metrics_rate_limit_resource_samples (mode);
