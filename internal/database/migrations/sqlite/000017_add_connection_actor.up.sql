alter table connections add column actor_id text;

create index idx_connections_actor_id
    on connections (actor_id);
