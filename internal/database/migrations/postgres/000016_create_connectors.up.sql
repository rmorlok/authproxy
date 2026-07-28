create table connectors
(
    id         text primary key,
    namespace  text not null,
    name       text not null check (name <> ''),
    created_at timestamptz not null,
    updated_at timestamptz not null,
    deleted_at timestamptz
);

create index idx_connectors_deleted_at
    on connectors (deleted_at);

create unique index idx_connectors_live_namespace_name
    on connectors (namespace, name)
    where deleted_at is null;

with ranked as (
    select
        id,
        namespace,
        row_number() over (
            partition by id
            order by
                case when deleted_at is null then 0 else 1 end,
                version
        ) as row_num,
        min(created_at) over (partition by id) as earliest_created_at,
        max(updated_at) over (partition by id) as latest_updated_at,
        sum(case when deleted_at is null then 1 else 0 end) over (partition by id) as live_versions,
        max(deleted_at) over (partition by id) as latest_deleted_at
    from connector_versions
)
insert into connectors (
    id,
    namespace,
    name,
    created_at,
    updated_at,
    deleted_at
)
select
    id,
    namespace,
    id,
    coalesce(earliest_created_at, current_timestamp),
    coalesce(latest_updated_at, earliest_created_at, current_timestamp),
    case when live_versions > 0 then null else latest_deleted_at end
from ranked
where row_num = 1;

alter table connector_versions drop column namespace;
