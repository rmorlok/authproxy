create table data_encryption_keys (
    id                text primary key,
    key_id            text not null,
    provider          text not null,
    provider_id       text not null,
    provider_version  text not null,
    provider_metadata jsonb,
    protected_data    jsonb not null,
    is_current        boolean not null default false,
    created_at        timestamptz not null,
    updated_at        timestamptz not null,
    deleted_at        timestamptz
);

create index idx_dek_scope_current on data_encryption_keys (deleted_at, key_id, is_current);
