create table tool_set_generations
(
    tool_set_id          text not null,
    generation           integer not null check (generation > 0),
    state                text not null check (state in ('draft', 'primary', 'active', 'archived')),
    encrypted_definition text not null,
    revision             integer not null check (revision > 0),
    created_at           datetime not null,
    updated_at           datetime not null,
    encrypted_at         datetime,
    deleted_at           datetime,
    primary key (tool_set_id, generation),
    foreign key (tool_set_id) references tool_sets (id) on delete restrict
);

create unique index idx_tool_set_generations_live_draft
    on tool_set_generations (tool_set_id)
    where deleted_at is null and state = 'draft';

create unique index idx_tool_set_generations_live_primary
    on tool_set_generations (tool_set_id)
    where deleted_at is null and state = 'primary';

create index idx_tool_set_generations_deleted_at
    on tool_set_generations (deleted_at);
