create table connection_credentials
(
    id                    text primary key,
    connection_id         text not null,
    encrypted_credentials jsonb,
    placement_snapshot    jsonb,
    created_by_actor_id   text,
    last_validated_at     timestamptz,
    created_at            timestamptz,
    encrypted_at          timestamptz,
    deleted_at            timestamptz
);

create index idx_connection_credentials_deleted_at
    on connection_credentials (deleted_at);

create index idx_connection_credentials_connection_active
    on connection_credentials (connection_id, deleted_at);
