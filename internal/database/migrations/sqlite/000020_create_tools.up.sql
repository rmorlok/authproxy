create table tools
(
    id              text primary key,
    namespace       text not null,
    name            text not null,
    connection_id   text not null,
    active_revision integer not null check (active_revision > 0),
    created_at      datetime not null,
    updated_at      datetime not null,
    deleted_at      datetime,
    foreign key (namespace) references namespaces (path) on delete cascade,
    foreign key (connection_id) references connections (id) on delete cascade,
    -- A tool and its first revision are inserted in one transaction.
    foreign key (id, active_revision) references tool_revisions (tool_id, revision)
        deferrable initially deferred
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
    tool_id              text not null,
    revision             integer not null check (revision > 0),
    -- Historical connection identity must survive later reassignment/purging.
    connection_id        text not null,
    encrypted_definition text not null,
    definition_digest    text not null check (length(definition_digest) = 64),
    created_at           datetime not null,
    encrypted_at         datetime,
    deleted_at           datetime,
    primary key (tool_id, revision),
    foreign key (tool_id) references tools (id) on delete cascade
);

create index idx_tool_revisions_deleted_at
    on tool_revisions (deleted_at);
