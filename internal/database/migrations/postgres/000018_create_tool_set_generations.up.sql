create table tool_set_generations
(
    tool_set_id          text not null references tool_sets (id) on delete restrict,
    generation           bigint not null check (generation > 0),
    state                text not null check (state in ('draft', 'primary', 'active', 'archived')),
    encrypted_definition jsonb not null,
    revision             bigint not null check (revision > 0),
    created_at           timestamptz not null,
    updated_at           timestamptz not null,
    encrypted_at         timestamptz,
    deleted_at           timestamptz,
    primary key (tool_set_id, generation)
);

create unique index idx_tool_set_generations_live_draft
    on tool_set_generations (tool_set_id)
    where deleted_at is null and state = 'draft';

create unique index idx_tool_set_generations_live_primary
    on tool_set_generations (tool_set_id)
    where deleted_at is null and state = 'primary';

create index idx_tool_set_generations_deleted_at
    on tool_set_generations (deleted_at);
