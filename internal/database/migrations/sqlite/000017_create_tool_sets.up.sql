create table tool_sets
(
    id                  text primary key,
    namespace           text not null,
    name                text not null check (name <> ''),
    labels              text,
    annotations         text,
    connection_selector text not null,
    selector_revision   integer not null check (selector_revision > 0),
    created_at          datetime not null,
    updated_at          datetime not null,
    deleted_at          datetime
);

create index idx_tool_sets_deleted_at
    on tool_sets (deleted_at);

create index idx_tool_sets_resource_search
    on tool_sets (deleted_at, updated_at desc, id);

create unique index idx_tool_sets_live_namespace_name
    on tool_sets (namespace, name)
    where deleted_at is null;
