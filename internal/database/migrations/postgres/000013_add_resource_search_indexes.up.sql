create index idx_actors_resource_search on actors (deleted_at, updated_at desc, id);
create index idx_connections_resource_search on connections (deleted_at, updated_at desc, id);
create index idx_connector_generations_resource_search on connector_generations (deleted_at, updated_at desc, id, generation);
create index idx_namespaces_resource_search on namespaces (deleted_at, updated_at desc, path);
create index idx_keys_resource_search on keys (deleted_at, updated_at desc, id);
create index idx_rate_limits_resource_search on rate_limits (deleted_at, updated_at desc, id);
