create table actors
(
    id            text primary key,
    namespace     text,
    labels        jsonb,
    annotations   jsonb,
    external_id   text,
    encrypted_key jsonb,
    permissions   jsonb,
    created_at    timestamptz,
    updated_at    timestamptz,
    encrypted_at  timestamptz,
    deleted_at    timestamptz
);

create index idx_actors_deleted_at
    on actors (deleted_at);

create unique index idx_actors_namespace
    on actors (namespace, external_id);

create table connections
(
    id                      text primary key,
    namespace               text,
    labels                  jsonb,
    annotations             jsonb,
    state                   text,
    connector_id            text,
    connector_version       integer,
    encrypted_configuration jsonb,
    setup_step              text,
    created_at              timestamptz,
    updated_at              timestamptz,
    encrypted_at            timestamptz,
    deleted_at              timestamptz
);

create index idx_connections_deleted_at
    on connections (deleted_at);

create table connector_versions
(
    id                   text,
    version              integer,
    namespace            text,
    labels               jsonb,
    annotations          jsonb,
    state                text,
    type                 text,
    hash                 text,
    encrypted_definition jsonb,
    created_at           timestamptz,
    updated_at           timestamptz,
    encrypted_at         timestamptz,
    deleted_at           timestamptz,
    primary key (id, version)
);

create index idx_connector_versions_deleted_at
    on connector_versions (deleted_at);

create table namespaces
(
    path                                   text primary key,
    depth                                  integer,
    state                                  text,
    key_id                                 text,
    target_data_encryption_key_id          text,
    target_data_encryption_key_updated_at  timestamptz,
    labels                                 jsonb,
    annotations                            jsonb,
    created_at                             timestamptz,
    updated_at                             timestamptz,
    deleted_at                             timestamptz
);

create index idx_namespaces_deleted_at
    on namespaces (deleted_at);

create table oauth2_tokens
(
    id                      text primary key,
    connection_id           text not null,
    refreshed_from_id       text,
    encrypted_refresh_token jsonb,
    encrypted_access_token  jsonb,
    access_token_expires_at timestamptz,
    scopes                  text,
    created_at              timestamptz,
    encrypted_at            timestamptz,
    deleted_at              timestamptz
);

create index idx_oauth2_tokens_deleted_at
    on oauth2_tokens (deleted_at);

create table used_nonces
(
    id           text primary key,
    retain_until timestamptz,
    created_at   timestamptz
);

create index idx_used_nonces_retain_until
    on used_nonces (retain_until);

create table keys (
    id                 text primary key,
    namespace          text not null,
    usage              text not null default 'data_encryption',
    material_type      text not null default 'symmetric',
    encrypted_key_data jsonb,
    state              text not null default 'active',
    labels             jsonb,
    annotations        jsonb,
    created_at         timestamptz not null,
    updated_at         timestamptz not null,
    encrypted_at       timestamptz,
    deleted_at         timestamptz
);

create index idx_keys_deleted_at on keys (deleted_at);
create index idx_keys_namespace on keys (deleted_at, namespace);

insert into keys (
    id,
    namespace,
    usage,
    material_type,
    encrypted_key_data,
    state,
    labels,
    created_at,
    updated_at,
    deleted_at
) values (
    'key_global',
    'root',
    'data_encryption',
    'symmetric',
    null,
    'active',
    '{}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    null
);
