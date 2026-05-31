create table connection_credentials
(
    id                    text primary key,
    connection_id         text not null,
    encrypted_credentials text,
    placement_snapshot    text,
    created_by_actor_id   text,
    last_validated_at     datetime,
    created_at            datetime,
    encrypted_at          datetime,
    deleted_at            datetime
);

create index idx_connection_credentials_deleted_at
    on connection_credentials (deleted_at);

create index idx_connection_credentials_connection_active
    on connection_credentials (connection_id, deleted_at);
