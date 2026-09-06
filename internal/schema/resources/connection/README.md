# Connection resource schema

This package owns the canonical `authproxy.net/v1alpha1` `Connection`
resource. Common identity, namespace, labels, annotations, and timestamps live
in `metadata`. `spec.connectorRef` pins the exact connector generation that
interprets the connection.

Connector-defined setup values appear under `spec.configuration`, flow through
typed setup actions, and are encrypted at rest. API reads return those values
in cleartext. Server-derived configuration state lives under
`status.configuration`: `configured` reports whether encrypted configuration
has been persisted, and `schema` contains an aggregate JSON Schema for the
top-level fields from connector-authored preconnect and configure forms. The
schema omits step-level `required` constraints so incomplete setup and skipped
conditional steps remain representable. Setup actions continue to validate
submissions against each step's original schema. The aggregate allows
additional properties because connector migration hooks may create
configuration fields without a form schema; those fields remain untyped.

Auth-method-emitted fields such as API keys, OAuth client credentials, access
tokens, and refresh tokens are persisted in dedicated encrypted credential
storage. They are not part of `spec.configuration` or its schema. Lifecycle,
aggregate credential/probe health, setup progress, and whether encrypted setup
configuration exists are server-owned observations under `status`.
Connector authors should use auth methods for secret material rather than
collecting it in custom setup fields, because connection read/list responses
return connector-authored configuration in cleartext.

CRUD updates use `ConnectionPatch`. Only mutable metadata may change through
that contract; connector migrations and setup transitions use their dedicated
API actions.
