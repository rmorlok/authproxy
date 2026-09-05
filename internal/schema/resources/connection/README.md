# Connection resource schema

This package owns the canonical `authproxy.net/v1alpha1` `Connection`
resource. Common identity, namespace, labels, annotations, and timestamps live
in `metadata`. `spec.connectorRef` pins the exact connector generation that
interprets the connection, while optional `spec.actorRef` records the actor
that initiated it without changing namespace-based authorization.

Connector-defined setup values appear under `spec.configuration`, flow through
typed setup actions, and are encrypted at rest. API serialization recursively
redacts their values. Connection configuration is write-only even when the
caller may replay secrets from other resource types.
Lifecycle, aggregate credential/probe health, setup progress, and whether
encrypted setup configuration exists are server-owned observations under
`status`.

CRUD updates use `ConnectionPatch`. Only mutable metadata may change through
that contract; connector migrations and setup transitions use their dedicated
API actions.
