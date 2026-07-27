alter table actors add column name text;
update actors set name = id;
alter table actors alter column name set not null;
alter table actors add constraint actors_name_nonempty check (name <> '');

alter table connections add column name text;
update connections set name = id;
alter table connections alter column name set not null;
alter table connections add constraint connections_name_nonempty check (name <> '');

alter table keys add column name text;
update keys set name = id;
alter table keys alter column name set not null;
alter table keys add constraint keys_name_nonempty check (name <> '');

alter table rate_limits add column name text;
update rate_limits set name = id;
alter table rate_limits alter column name set not null;
alter table rate_limits add constraint rate_limits_name_nonempty check (name <> '');

create unique index idx_actors_live_namespace_name
    on actors (namespace, name)
    where deleted_at is null;

create unique index idx_connections_live_namespace_name
    on connections (namespace, name)
    where deleted_at is null;

create unique index idx_keys_live_namespace_name
    on keys (namespace, name)
    where deleted_at is null;

create unique index idx_rate_limits_live_namespace_name
    on rate_limits (namespace, name)
    where deleted_at is null;
