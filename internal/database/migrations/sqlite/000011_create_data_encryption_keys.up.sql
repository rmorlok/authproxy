create table data_encryption_keys (
    id                text primary key,
    key_id            text not null,
    provider          text not null,
    provider_id       text not null,
    provider_version  text not null,
    provider_metadata text,
    protected_data    text not null,
    is_current        integer not null default 0,
    created_at        datetime not null,
    updated_at        datetime not null,
    deleted_at        datetime
);

create index idx_dek_scope_current on data_encryption_keys (deleted_at, key_id, is_current);
