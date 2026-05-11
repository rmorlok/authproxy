create table api_key_credentials
(
    id                    text primary key,
    connection_id         text not null,
    encrypted_api_key     jsonb,
    encrypted_username    jsonb,
    placement_snapshot    jsonb,
    created_by_actor_id   text,
    last_validated_at     timestamptz,
    created_at            timestamptz,
    encrypted_at          timestamptz,
    deleted_at            timestamptz
);

create index idx_api_key_credentials_deleted_at
    on api_key_credentials (deleted_at);

create index idx_api_key_credentials_connection_active
    on api_key_credentials (connection_id, deleted_at);
