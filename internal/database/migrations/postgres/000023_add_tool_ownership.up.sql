-- Ownership survives soft deletion so a source key can recover its original
-- Tool identity within the same binding lifetime.
alter table tools add column binding_id text references tool_set_bindings (id) on delete cascade;
alter table tools add column source_key text not null default ''
    check ((binding_id is null and source_key = '')
        or (binding_id is not null and length(trim(source_key)) > 0));

-- Include tombstones in uniqueness. The leading binding column also supports
-- the foreign-key lookup when a binding is physically removed.
create unique index idx_tools_binding_source_key on tools (binding_id, source_key);
