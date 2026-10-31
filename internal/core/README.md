# Core

This package implements the central business logic of auth proxy for managing connectors and connections. This
package wraps the database and redis to efficiently load fully hydrated models that expose method for interacting with
the system that will handle appropriate logging an event queuing for background notifications and tasks.

Generally speaking, other packages should not take dependencies on core directly, but rather take dependencies on its
`iface` subpackage that exposes the core interfaces as well as some of the data model used by those interfaces.

Kubernetes-style `ObjectReference` values are resolved through the typed
`Resolve<Resource>Reference` methods. Database resolution establishes the
logical resource identity; core is responsible for domain conversion and
hydration, such as selecting a connector definition generation or loading the
connector generation pinned by a connection.

## Tool invocation preparation

The private Tool admission helpers authorize and prepare one captured revision
for execution. Authorization requires one declared verb allowed by both the
actor and any JWT restrictions on the bound connection. Preparation then uses
the captured connection configuration and connector generation to supply a
template context and a connection-scoped HTTP client.

For example, a Tool may render `/{{path.cfg.tenant}}/contacts/{{path.params.id}}`.
If admission captured tenant `acme` and connector generation 2, that invocation
keeps those values even if the connection is later edited or migrated. Caller
arguments become `params` only after the executor validates them. The client
also retains the actor identity and labels captured during authorization.

Only a `configured` connection can be prepared. Unhealthy probe results and an
active setup step do not by themselves block a configured connection: its
current configuration can remain usable during reconfiguration or
reauthentication. Unsupported authentication methods fail preparation;
credentials are resolved later through the normal proxy lifecycle.

Preparation performs no upstream requests or schema evaluation. It decrypts
the captured connector and configuration with the caller's context and leaves
credential handling to the HTTP client. The caller retains the authorized
capture when preparation fails so a later result can identify the exact Tool
revision and connection. Executor orchestration and the public invocation route
are not wired yet.
