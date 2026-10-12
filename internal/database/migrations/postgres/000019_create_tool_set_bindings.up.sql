create table tool_set_bindings
(
    id            text primary key,
    tool_set_id   text not null references tool_sets (id) on delete restrict,
    -- Connection purging owns removal of any remaining bindings.
    connection_id text not null references connections (id) on delete cascade,
    epoch         bigint not null check (epoch > 0),
    created_at    timestamptz not null,
    updated_at    timestamptz not null,
    deleted_at    timestamptz
);

create unique index idx_tool_set_bindings_live_pair
    on tool_set_bindings (tool_set_id, connection_id)
    where deleted_at is null;

create index idx_tool_set_bindings_connection_inventory
    on tool_set_bindings (connection_id, deleted_at, id);

create index idx_tool_set_bindings_deleted_at
    on tool_set_bindings (deleted_at);
