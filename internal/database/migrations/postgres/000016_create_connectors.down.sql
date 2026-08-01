create table connector_versions
(
    id                   text,
    version              bigint,
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
    primary key (id, version)
);

insert into connector_versions (
    id,
    version,
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
    d.version,
    c.namespace,
    c.labels,
    c.annotations,
    d.state,
    null,
    '',
    d.encrypted_definition,
    c.created_at,
    c.updated_at,
    d.encrypted_at,
    d.deleted_at
from connector_definition_versions d
join connectors c on c.id = d.connector_id;

create index idx_connector_versions_deleted_at
    on connector_versions (deleted_at);

create index idx_connector_versions_resource_search
    on connector_versions (deleted_at, updated_at desc, id, version);

drop table connector_definition_versions;
drop table connectors;
