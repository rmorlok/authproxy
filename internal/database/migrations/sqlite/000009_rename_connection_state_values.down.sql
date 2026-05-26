-- Reverse the value rewrite from the up migration.
update connections
set state = 'created'
where state = 'setup';

update connections
set state = 'ready'
where state = 'configured';
