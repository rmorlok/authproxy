create table rate_limits (
    id          text primary key,
    namespace   text not null,
    definition  text not null,
    labels      text,
    annotations text,
    created_at  datetime not null,
    updated_at  datetime not null,
    deleted_at  datetime
);

create index idx_rate_limits_deleted_at on rate_limits (deleted_at);
create index idx_rate_limits_namespace on rate_limits (deleted_at, namespace);
