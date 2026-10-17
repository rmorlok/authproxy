-- Pending assignments have no applied generation or inventory. These fields
-- move together when a complete prepared inventory is installed atomically.
alter table tool_set_bindings add column applied_generation integer;
alter table tool_set_bindings add column inventory_revision integer not null default 0
    check ((applied_generation is null and inventory_revision = 0)
        or (applied_generation is not null and applied_generation > 0 and inventory_revision > 0));

-- The owning Tool's immutable binding identifies the source ToolSet. Preserve
-- the exact source generation on each revision independently of later installs.
alter table tool_revisions add column source_generation integer
    check (source_generation > 0);
