# Rate Limit Resource Schema

This package defines rate-limit resource configuration, including selectors, buckets, and algorithms.

The resource shape here is shared by config, persistence-facing code, rate-limit matching/enforcement, and API DTOs. Keep API envelopes, pagination responses, and dry-run request/response contracts in `internal/schema/api`; this package should only describe the reusable rate-limit definition.
