create table legacy_connector_generations
(
    id                   text,
    generation           bigint,
    namespace            text,
    labels               jsonb,
    annotations          jsonb,
    state                text,
    type                 text,
    hash                 text,
    encrypted_definition jsonb,
    created_at           timestamptz,
    updated_at           timestamptz,
    encrypted_at         timestamptz,
    deleted_at           timestamptz,
    primary key (id, generation)
);

insert into legacy_connector_generations (
    id,
    generation,
    namespace,
    labels,
    annotations,
    state,
    type,
    hash,
    encrypted_definition,
    created_at,
    updated_at,
    encrypted_at,
    deleted_at
)
select
    d.connector_id,
    d.generation,
    c.namespace,
    c.labels,
    c.annotations,
    d.state,
    null,
    '',
    d.encrypted_definition,
    d.created_at,
    d.updated_at,
    d.encrypted_at,
    d.deleted_at
from connector_generations d
join connectors c on c.id = d.connector_id;

drop table connector_generations;
alter table legacy_connector_generations rename to connector_generations;

create index idx_connector_generations_deleted_at
    on connector_generations (deleted_at);

create index idx_connector_generations_resource_search
    on connector_generations (deleted_at, updated_at desc, id, generation);

drop table connectors;
