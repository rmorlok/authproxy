create table rate_limits (
    id          text primary key,
    namespace   text not null,
    definition  jsonb not null,
    labels      jsonb,
    annotations jsonb,
    created_at  timestamptz not null,
    updated_at  timestamptz not null,
    deleted_at  timestamptz
);

create index idx_rate_limits_deleted_at on rate_limits (deleted_at);
create index idx_rate_limits_namespace on rate_limits (deleted_at, namespace);
