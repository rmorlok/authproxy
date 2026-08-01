create table connectors
(
    id          text primary key,
    namespace   text not null,
    name        text not null check (name <> ''),
    labels      text,
    annotations text,
    created_at  datetime not null,
    updated_at  datetime not null,
    deleted_at  datetime
);

create index idx_connectors_deleted_at
    on connectors (deleted_at);

create index idx_connectors_resource_search
    on connectors (deleted_at, updated_at desc, id);

create unique index idx_connectors_live_namespace_name
    on connectors (namespace, name)
    where deleted_at is null;

with ranked as (
    select
        id,
        namespace,
        labels,
        annotations,
        row_number() over (
            partition by id
            order by
                case when deleted_at is null then 0 else 1 end,
                case state
                    when 'primary' then 1
                    when 'draft' then 2
                    when 'active' then 3
                    when 'archived' then 4
                    else 5
                end,
                version desc
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
    labels,
    annotations,
    created_at,
    updated_at,
    deleted_at
)
select
    id,
    namespace,
    id,
    labels,
    annotations,
    coalesce(earliest_created_at, current_timestamp),
    coalesce(latest_updated_at, earliest_created_at, current_timestamp),
    case when live_versions > 0 then null else latest_deleted_at end
from ranked
where row_num = 1;

create table connector_definition_versions
(
    id                   text primary key,
    connector_id         text not null,
    version              integer not null,
    state                text not null,
    encrypted_definition text not null,
    created_at           datetime not null,
    updated_at           datetime not null,
    encrypted_at         datetime,
    deleted_at           datetime,
    foreign key (connector_id) references connectors (id) on delete cascade,
    unique (connector_id, version)
);

create index idx_connector_definition_versions_connector_state
    on connector_definition_versions (connector_id, state);

create index idx_connector_definition_versions_deleted_at
    on connector_definition_versions (deleted_at);

insert into connector_definition_versions (
    id,
    connector_id,
    version,
    state,
    encrypted_definition,
    created_at,
    updated_at,
    encrypted_at,
    deleted_at
)
select
    'cvd_' || substr(cv.id, 5) || '_' || cast(cv.version as text),
    cv.id,
    cv.version,
    cv.state,
    cv.encrypted_definition,
    coalesce(cv.created_at, c.created_at, current_timestamp),
    coalesce(cv.updated_at, cv.created_at, c.updated_at, c.created_at, current_timestamp),
    cv.encrypted_at,
    c.deleted_at
from connector_versions cv
join connectors c on c.id = cv.id;

drop table connector_versions;
