# Actor resource schema

This package owns the canonical `authproxy.net/v1alpha1` `Actor` resource.
Actor identity and namespace live in `metadata`; the host application's external
subject, base permissions, and optional write-only signing-key configuration
live in `spec`.

Signing material is accepted when configuring or creating an actor, encrypted
before persistence, and never returned from the core or HTTP API. Responses use
`status.signingKeyConfigured` to report whether actor-specific verification
material exists without exposing it.

JWT policy does not belong in this package. JWT claims may reuse the Actor shape
with a stricter validation policy that forbids database identity, timestamps,
status, and signing-key material.
