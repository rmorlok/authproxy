alter table connector_versions add column namespace text;

update connector_versions
set namespace = (
    select connectors.namespace
    from connectors
    where connectors.id = connector_versions.id
);

drop index if exists idx_connectors_live_namespace_name;
drop index if exists idx_connectors_deleted_at;
drop table connectors;
