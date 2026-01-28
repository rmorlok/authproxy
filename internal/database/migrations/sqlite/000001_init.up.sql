create table actors
(
    id          text primary key,
    namespace   text,
    external_id text,
    email       text,
    admin       numeric,
    super_admin numeric,
    permissions text,
    created_at  datetime,
    updated_at  datetime,
    deleted_at  datetime
);

create index idx_actors_deleted_at
    on actors (deleted_at);

create index idx_actors_email
    on actors (email);

create unique index idx_actors_namespace
    on actors (namespace, external_id);

create table connections
(
    id                text primary key,
    namespace         text,
    state             text,
    connector_id      text,
    connector_version integer,
    created_at        datetime,
    updated_at        datetime,
    deleted_at        datetime
);

create index idx_connections_deleted_at
    on connections (deleted_at);

create table connector_versions
(
    id                   text,
    version              integer,
    namespace            text,
    state                text,
    type                 text,
    hash                 text,
    encrypted_definition text,
    created_at           datetime,
    updated_at           datetime,
    deleted_at           datetime,
    primary key (id, version)
);

create index idx_connector_versions_deleted_at
    on connector_versions (deleted_at);

create table namespaces
(
    path       text primary key,
    depth      integer,
    state      text,
    created_at datetime,
    updated_at datetime,
    deleted_at datetime
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
    access_token_expires_at datetime,
    scopes                  text,
    created_at              datetime,
    deleted_at              datetime
);

create index idx_oauth2_tokens_deleted_at
    on oauth2_tokens (deleted_at);

create table used_nonces
(
    id           text primary key,
    retain_until datetime,
    created_at   datetime
);

create index idx_used_nonces_retain_until
    on used_nonces (retain_until);

