create table connector_versions
(
    id                   text,
    version              integer,
    namespace            text,
    labels               text,
    annotations          text,
    state                text,
    type                 text,
    hash                 text,
    encrypted_definition text,
    created_at           datetime,
    updated_at           datetime,
    encrypted_at         datetime,
    deleted_at           datetime,
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
    d.created_at,
    d.updated_at,
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
