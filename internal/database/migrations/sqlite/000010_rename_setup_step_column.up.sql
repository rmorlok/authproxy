-- Rename connections.setup_step -> connections.setup_step_id to reflect that
-- the column now stores a raw step identifier (the user-authored YAML step id
-- or an apxy:* pseudo-step id) rather than the prior "phase:index" encoding.
alter table connections
    rename column setup_step to setup_step_id;
