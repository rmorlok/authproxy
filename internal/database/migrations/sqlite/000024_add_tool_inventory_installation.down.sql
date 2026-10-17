-- Downgrading intentionally removes generated inventories and their cascaded
-- revision history before forgetting installation state and source provenance.
-- Keep standalone Tools and binding identities/epochs; leaving owned Tool rows
-- behind would block a fresh installation after a subsequent upgrade.
delete from tools where binding_id is not null;
alter table tool_revisions drop column source_generation;
alter table tool_set_bindings drop column inventory_revision;
alter table tool_set_bindings drop column applied_generation;
