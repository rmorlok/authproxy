-- Rename ConnectionState values: 'created' -> 'setup', 'ready' -> 'configured'.
-- Free-text column, no CHECK constraints — this is purely a value rewrite so
-- existing rows continue to load correctly via IsValidConnectionState.
update connections
set state = 'setup'
where state = 'created';

update connections
set state = 'configured'
where state = 'ready';
