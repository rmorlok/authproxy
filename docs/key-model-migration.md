# Unified Key Model Migration

This document is the design contract for the key model migration tracked by
[#605](https://github.com/rmorlok/authproxy/issues/605). The feature branch for
the full migration is `codex/key-model-migration`.

## Branching

- The long-lived branch is `codex/key-model-migration`.
- The long-lived PR targets `main` and stays in draft until the migration is complete.
- Child PRs branch from `codex/key-model-migration` and target `codex/key-model-migration`.
- After all child PRs land and the full migration is validated, the long-lived PR merges to `main`.

## Migration Stance

This migration is not backwards compatible. No production environment is running
the current encryption model, so implementation PRs may use forward migrations to
rename, drop, or recreate tables. Demo and development environments can be
destroyed and recreated when this branch merges.

## Why The Model Changes

The legacy model evolved from one global AES key into three database layers:

- `encryption_keys`
- `encryption_key_versions`
- `data_encryption_keys`

That shape makes cloud KMS support look like a special case of key versions. The
target model is a simpler envelope-encryption model:

- `keys` tracks the logical keys AuthProxy knows about.
- `data_encryption_keys` tracks the DEKs that encrypt stored application data.
- Provider versions remain provider metadata on DEKs, not database rows.

## Terminology

| Term | Meaning |
| --- | --- |
| Key | A logical key record in `keys`, using a `key_` id. |
| DEK | A data encryption key in `data_encryption_keys`, using a `dek_` id. |
| Wrapping key | Provider-managed material used to protect a DEK. It may live in KMS, a secret manager, an environment variable, a file, or config. |
| Provider version | Version metadata from the wrapping provider. It is stored on the DEK so AuthProxy can detect stale wrapping. |
| Target DEK | The current DEK a namespace should use for new writes and re-encryption. |

## Target Data Model

```mermaid
erDiagram
    keys ||--o{ data_encryption_keys : owns
    keys ||--o{ namespaces : selected_by
    data_encryption_keys ||--o{ namespaces : target_for
    data_encryption_keys ||--o{ encrypted_fields : referenced_by

    keys {
        string id "key_..."
        string namespace
        string usage
        string material_type
        string state
        json encrypted_key_data
        json labels
        json annotations
        timestamp encrypted_at
        timestamp created_at
        timestamp updated_at
        timestamp deleted_at
    }

    data_encryption_keys {
        string id "dek_..."
        string key_id
        string provider
        string provider_id
        string provider_version
        json provider_metadata
        json protected_data
        bool is_current
        timestamp created_at
        timestamp updated_at
        timestamp deleted_at
    }

    namespaces {
        string path
        string key_id
        string target_data_encryption_key_id
    }

    encrypted_fields {
        string id "dek_..."
        string d "ciphertext"
    }
```

### `keys`

`keys` replaces `encryption_keys` as the general key registry. It is not limited
to data encryption; future key usages can include public keys used to validate
JWT signatures.

Initial required fields:

- `id`: uses the `key_` prefix. The global key becomes `key_global`.
- `namespace`: namespace ownership and inheritance boundary.
- `usage`: why the key exists. The initial usage is data encryption.
- `material_type`: the kind of backing material, such as symmetric, public,
  private, or external provider-managed material.
- `state`: lifecycle state, initially active or disabled.
- `encrypted_key_data`: encrypted provider configuration for data-encryption
  wrapping keys. Future non-secret public-key records do not need to use this
  field if their material is intentionally public.

The migration should rename API, core, SDK, Terraform, and route language from
`encryption_key` to `key`.

### `data_encryption_keys`

`data_encryption_keys` becomes the runtime encryption material table. A DEK is
the key used by AES-GCM to encrypt application data in the database.

Each DEK belongs to exactly one `keys.id` through `key_id`. The DEK row stores
the protected key material and enough provider metadata to determine whether it
is still wrapped with the latest provider material.

Required behavior:

- `id` uses the `dek_` prefix.
- `key_id` references `keys.id`.
- `protected_data` stores wrapped or encrypted DEK material, never plaintext.
- `provider`, `provider_id`, `provider_version`, and `provider_metadata` record
  the wrapping provider state used to protect the DEK.
- `is_current` selects the DEK used for new writes under that key.

Plaintext DEKs may be cached in memory by the encrypt service, but plaintext DEK
material must not be stored in the database.

### Removed Table

`encryption_key_versions` goes away. Provider versions are not AuthProxy key
versions. They are wrapping metadata on each DEK.

## Encrypted Field Format

`encfield.EncryptedField` keeps the same JSON shape:

```json
{"id":"dek_abc123","d":"base64-encoded-ciphertext"}
```

The meaning of `id` changes:

- Removed model: `id` was an `encryption_key_versions.id` value, usually `ekv_...`.
- Target model: `id` is a `data_encryption_keys.id` value, always `dek_...`.

This keeps encrypted fields self-describing while making the database DEK row the
unit of encryption, cache lookup, and re-encryption.

## Namespace Resolution

Namespaces continue to choose a logical key by inheritance, but the column names
change:

- `encryption_key_id` becomes `key_id`.
- `target_encryption_key_version_id` becomes `target_data_encryption_key_id`.

Resolution flow:

```mermaid
flowchart TD
    A["Write data for namespace"] --> B["Walk namespace path to find nearest key_id"]
    B --> C["Load current DEK for key_id"]
    C --> D["Encrypt with plaintext DEK"]
    D --> E["Store EncryptedField.ID as dek_id"]

    F["Re-encryption task"] --> G["Load namespace target_data_encryption_key_id"]
    G --> H["Scan registered encrypted fields"]
    H --> I{"EncryptedField.ID matches target DEK?"}
    I -- "yes" --> J["Leave row unchanged"]
    I -- "no" --> K["Decrypt with old DEK, encrypt with target DEK"]
```

Changing a namespace target DEK drives the same re-encryption behavior that key
versions drive today: rows in that namespace are checked and moved to the target
DEK when needed.

## Provider Model

The provider abstraction should separate logical key metadata from DEK
generation and wrapping:

- Secret-backed providers can expose wrapping material to AuthProxy. AuthProxy
  generates a strong random DEK, wraps or encrypts it with that provider-backed
  material, and stores the protected DEK.
- KMS-style providers keep wrapping material outside AuthProxy. AuthProxy asks
  the provider to generate a DEK when the provider has a native data-key API.
- When a provider cannot generate data keys, AuthProxy generates a strong random
  DEK and asks the provider to wrap or encrypt it.
- All providers must be able to unwrap or decrypt an existing DEK so AuthProxy
  can decrypt data and rewrap stale DEKs.

The provider interface introduced by implementation PRs should support these
operations:

- Describe the latest wrapping metadata for a key.
- Generate a DEK using provider-native data-key generation when available.
- Wrap an AuthProxy-generated DEK when native generation is unavailable.
- Unwrap an existing protected DEK.
- Report enough metadata to decide whether an existing DEK is wrapped by the
  latest provider material.

## DEK Lifecycle

### Generation And Rotation

A dedicated DEK generation process owns creating new DEKs. It walks active data
encryption keys and applies the configured DEK rotation policy.

Required behavior:

- Ensure each active data-encryption key has a current DEK.
- Rotate DEKs according to configuration.
- Prefer provider-native DEK generation when supported.
- Fall back to AuthProxy-generated strong randomness when native generation is
  unavailable.
- Mark the generated DEK current and leave older DEKs available for decryption.

### Rewrap Sync

The old "sync key versions to the database" task changes purpose. It no longer
manifests provider versions as rows. Instead, it inspects DEKs and checks whether
each current and retained DEK is wrapped using the latest provider material.

Required behavior:

- Load DEKs for each key.
- Compare stored provider metadata against the provider's latest wrapping
  metadata.
- If a DEK is stale, unwrap it, rewrap it with latest provider material, and save
  the updated protected data and metadata.
- Keep the DEK id stable during rewrap so encrypted fields do not need to change.

### Cache Sync

The encrypt service cache continues to exist, keyed by DEK id.

Cache entries should map:

```text
dek_id -> plaintext DEK bytes and metadata needed for encryption/decryption
```

## API And Generated Artifacts

Public naming changes from encryption keys to keys:

- Route family: `/encryption-keys` becomes `/keys`.
- Request and response fields: `encryption_key_id` becomes `key_id`.
- Namespace fields: `target_encryption_key_version_id` becomes
  `target_data_encryption_key_id`.
- JS SDK, Terraform provider, OpenAPI, generated docs, and tests follow the new
  names.

## Integration Testing

Provider implementation PRs must add integration tests for the KMS providers in
the project:

- AWS KMS
- Google Cloud KMS
- HashiCorp Vault Transit

The provider tests should prove:

- DEK creation works using provider-native generation when available.
- Fallback generation plus provider wrapping works when native generation is not
  available.
- Existing DEKs can be unwrapped for decryption.
- A stale DEK can be rewrapped with updated provider metadata while keeping the
  same `dek_` id.
- Encrypted application data uses `dek_` ids and remains decryptable after
  provider rotation and DEK rewrap.

AWS KMS integration tests require an existing symmetric KMS key because KMS keys
cannot be deleted immediately after creation. Set `AUTH_PROXY_AWS_KMS_KEY_ID_V2`
to a second accessible key ID or alias to exercise metadata advancement and
rewrap under new provider material.

## Implementation Order

The child issues under #605 are intended to land in this rough order:

1. Establish the branch and this design contract.
2. Replace the database schema and core naming from encryption keys to keys.
3. Make DEKs the runtime encryption material.
4. Introduce provider generation, wrap, unwrap, and metadata abstractions.
5. Move encrypt service caching, namespace targeting, and re-encryption to DEKs.
6. Replace key-version sync with DEK rewrap sync.
7. Remove all remaining key-version code and generated artifacts.
8. Add KMS provider implementations and integration tests.
9. Update operational docs.
