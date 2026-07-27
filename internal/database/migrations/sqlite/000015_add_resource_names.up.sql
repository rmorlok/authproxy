alter table actors add column name text not null default '';
update actors set name = id;

alter table connections add column name text not null default '';
update connections set name = id;

alter table keys add column name text not null default '';
update keys set name = id;

alter table rate_limits add column name text not null default '';
update rate_limits set name = id;

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

create trigger actors_name_required_insert
before insert on actors
when new.name = ''
begin
    select raise(abort, 'actors.name is required');
end;

create trigger actors_name_required_update
before update of name on actors
when new.name = ''
begin
    select raise(abort, 'actors.name is required');
end;

create trigger connections_name_required_insert
before insert on connections
when new.name = ''
begin
    select raise(abort, 'connections.name is required');
end;

create trigger connections_name_required_update
before update of name on connections
when new.name = ''
begin
    select raise(abort, 'connections.name is required');
end;

create trigger keys_name_required_insert
before insert on keys
when new.name = ''
begin
    select raise(abort, 'keys.name is required');
end;

create trigger keys_name_required_update
before update of name on keys
when new.name = ''
begin
    select raise(abort, 'keys.name is required');
end;

create trigger rate_limits_name_required_insert
before insert on rate_limits
when new.name = ''
begin
    select raise(abort, 'rate_limits.name is required');
end;

create trigger rate_limits_name_required_update
before update of name on rate_limits
when new.name = ''
begin
    select raise(abort, 'rate_limits.name is required');
end;
