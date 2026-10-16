-- Existing encrypted definitions cannot be projected in SQL. NULL verbs mark
-- rows whose description and verbs still need to be prepared from the definition.
alter table tools add column description text not null default '';
alter table tools add column verbs text;
