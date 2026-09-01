# AGENTS.md

This package owns reusable namespace resource semantics.

Keep namespace path and matcher validation here, along with constants and helpers that describe the namespace hierarchy. Routes use this package's canonical resource and patch types directly when those are the complete wire contract. Endpoint-specific API envelopes belong in `internal/schema/api`, and auth-specific permission contracts belong in `internal/schema/auth`.

When adding a new namespace primitive, update `schema.json`, schema tests, and README guidance in the same change.
