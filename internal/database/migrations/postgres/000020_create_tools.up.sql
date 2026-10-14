create table tools
(
    id              text primary key,
    namespace       text not null references namespaces (path) on delete cascade,
    name            text not null,
    connection_id   text not null references connections (id) on delete cascade,
    active_revision bigint not null check (active_revision > 0),
    created_at      timestamptz not null,
    updated_at      timestamptz not null,
    deleted_at      timestamptz
);

create unique index idx_tools_live_namespace_name
    on tools (namespace, name)
    where deleted_at is null;

create index idx_tools_connection_inventory
    on tools (connection_id, deleted_at, id);

create index idx_tools_deleted_at
    on tools (deleted_at);

create table tool_revisions
(
    tool_id              text not null references tools (id) on delete cascade,
    revision             bigint not null check (revision > 0),
    -- Historical connection identity must survive later reassignment/purging.
    connection_id        text not null,
    encrypted_definition jsonb not null,
    definition_digest    text not null check (length(definition_digest) = 64),
    created_at           timestamptz not null,
    encrypted_at         timestamptz,
    deleted_at           timestamptz,
    primary key (tool_id, revision)
);

create index idx_tool_revisions_deleted_at
    on tool_revisions (deleted_at);

-- Defer the circular reference until a tool and its first revision both exist.
alter table tools
    add constraint fk_tools_active_revision
    foreign key (id, active_revision) references tool_revisions (tool_id, revision)
    deferrable initially deferred;
