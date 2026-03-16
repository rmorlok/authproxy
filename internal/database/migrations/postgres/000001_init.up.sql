create table actors
(
    id            text primary key,
    namespace     text,
    labels        text,
    external_id   text,
    encrypted_key text,
    permissions   text,
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
    id                text primary key,
    namespace         text,
    labels            text,
    state             text,
    connector_id      text,
    connector_version integer,
    created_at        timestamptz,
    updated_at        timestamptz,
    deleted_at        timestamptz
);

create index idx_connections_deleted_at
    on connections (deleted_at);

create table connector_versions
(
    id                   text,
    version              integer,
    namespace            text,
    labels               text,
    state                text,
    type                 text,
    hash                 text,
    encrypted_definition text,
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
    encryption_key_id                      text,
    target_encryption_key_version_id       text,
    target_encryption_key_version_updated_at timestamptz,
    labels                                 text,
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
    encrypted_refresh_token text,
    encrypted_access_token  text,
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

create table encryption_keys (
    id                 text primary key,
    namespace          text not null,
    encrypted_key_data text,
    state              text not null default 'active',
    labels             text,
    created_at         timestamptz not null,
    updated_at         timestamptz not null,
    encrypted_at       timestamptz,
    deleted_at         timestamptz
);

create index idx_encryption_keys_deleted_at on encryption_keys (deleted_at);
create index idx_encryption_keys_namespace on encryption_keys (deleted_at, namespace);

insert into encryption_keys (
    id,
    namespace,
    encrypted_key_data,
    state,
    labels,
    created_at,
    updated_at,
    deleted_at
) values (
    'ek_global',
    'root',
    null,
    'active',
    '{}',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP,
    null
);

create table encryption_key_versions (
    id                text primary key,
    encryption_key_id text not null,
    provider          text not null,
    provider_id       text not null,
    provider_version  text not null,
    ordered_version   integer not null,
    is_current        boolean not null default false,
    created_at        timestamptz not null,
    updated_at        timestamptz not null,
    deleted_at        timestamptz
);

create index idx_ekv_scope_current on encryption_key_versions (deleted_at, encryption_key_id, is_current);
create unique index idx_ekv_scope_ordered_version on encryption_key_versions (encryption_key_id, ordered_version);