create table connection_probe_outcomes
(
    id            text primary key,
    connection_id text not null,
    probe_id      text not null,
    outcome       text not null,
    error_message text,
    occurred_at   datetime not null,
    created_at    datetime not null
);

-- Lookup index for the read pattern: "most recent N outcomes for (connection, probe)"
-- used by the runtime to walk consecutive matching outcomes.
create index idx_connection_probe_outcomes_lookup
    on connection_probe_outcomes (connection_id, probe_id, occurred_at desc);
