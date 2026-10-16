-- Existing rows receive their materialized labels during label reconciliation.
alter table tools add column labels text;
alter table tools add column annotations text;

-- Namespace purging must find all referencing rows, including tombstones.
create index idx_tools_namespace
    on tools (namespace);
