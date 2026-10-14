-- Existing rows receive their materialized labels during label reconciliation.
alter table tools add column labels jsonb;
alter table tools add column annotations jsonb;

-- Namespace purging must find all referencing rows, including tombstones.
create index idx_tools_namespace
    on tools (namespace);
