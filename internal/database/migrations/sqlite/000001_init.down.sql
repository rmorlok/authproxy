-- Note that go-migrate will refuse to actually migrate down at this revision, so it won't invoke this.
drop index main.idx_actors_deleted_at;
drop index main.idx_actors_email;
drop table main.actors;

drop index main.idx_connections_deleted_at;
drop table main.connections;

drop index main.idx_connector_versions_deleted_at;
drop table main.connector_versions;

drop index main.idx_namespaces_deleted_at;
drop table main.namespaces;

drop index main.idx_oauth2_tokens_deleted_at;
drop table main.oauth2_tokens;

drop index main.idx_used_nonces_retain_until;
drop table main.used_nonces;

