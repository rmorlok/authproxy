create table connection_probe_health
(
    connection_id         text not null,
    probe_id              text not null,
    consecutive_failures  integer not null default 0,
    consecutive_successes integer not null default 0,
    last_outcome          text,
    last_outcome_at       datetime,
    created_at            datetime,
    updated_at            datetime,
    primary key (connection_id, probe_id)
);

create index idx_connection_probe_health_connection
    on connection_probe_health (connection_id);
