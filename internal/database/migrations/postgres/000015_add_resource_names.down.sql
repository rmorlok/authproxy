drop index if exists idx_actors_live_namespace_name;
drop index if exists idx_connections_live_namespace_name;
drop index if exists idx_keys_live_namespace_name;
drop index if exists idx_rate_limits_live_namespace_name;

alter table actors drop column name;
alter table connections drop column name;
alter table keys drop column name;
alter table rate_limits drop column name;
