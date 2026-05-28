CREATE TABLE IF NOT EXISTS app_metrics_connection_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'connection',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL,
    health_state TEXT NOT NULL,
    connector_id TEXT NOT NULL,
    connector_version INTEGER NOT NULL DEFAULT 0,
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_connection_resource_samples_namespace ON app_metrics_connection_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_connection_resource_samples_sampled_at ON app_metrics_connection_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_connection_resource_samples_connector ON app_metrics_connection_resource_samples (connector_id, connector_version);
CREATE INDEX IF NOT EXISTS idx_connection_resource_samples_state ON app_metrics_connection_resource_samples (state, health_state);

CREATE TABLE IF NOT EXISTS app_metrics_actor_resource_samples (
    sampled_at_ms INTEGER NOT NULL,
    resource_type TEXT NOT NULL DEFAULT 'actor',
    resource_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    labels TEXT NOT NULL DEFAULT '{}',
    external_id TEXT NOT NULL DEFAULT '',
    resource_created_at_ms INTEGER NOT NULL,
    resource_updated_at_ms INTEGER NOT NULL,
    resource_deleted_at_ms INTEGER,
    ingested_at_unix_nano INTEGER NOT NULL,
    PRIMARY KEY (sampled_at_ms, resource_id)
);

CREATE INDEX IF NOT EXISTS idx_actor_resource_samples_namespace ON app_metrics_actor_resource_samples (namespace);
CREATE INDEX IF NOT EXISTS idx_actor_resource_samples_sampled_at ON app_metrics_actor_resource_samples (sampled_at_ms);
CREATE INDEX IF NOT EXISTS idx_actor_resource_samples_external_id ON app_metrics_actor_resource_samples (external_id);
