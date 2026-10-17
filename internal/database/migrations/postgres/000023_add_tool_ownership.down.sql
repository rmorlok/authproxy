-- Downgrading intentionally removes generated Tools and their cascaded revision
-- history. Dropping ownership alone would make them editable standalone Tools.
-- Independently authored Tools and their revisions are preserved.
delete from tools where binding_id is not null;
drop index idx_tools_binding_source_key;
alter table tools drop column source_key;
alter table tools drop column binding_id;
