# Key Resource Schema

This package defines reusable key resource contracts, including signing keys,
key-data providers, and provider/runtime metadata used for data-encryption-key
wrapping.

Config, persistence-facing code, encryption services, and API DTOs can compose
these key resource types. Keep API envelopes, pagination responses, and
route-specific request/response contracts in `internal/schema/api`; this
package should only describe the reusable key definitions.
