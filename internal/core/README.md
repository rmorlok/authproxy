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

## Tool execution policy

Core translates `tools.invocation` configuration into complete operator limits
for the HTTP and JavaScript Tool executors. Omitted settings receive finite
defaults, explicit invalid values fail, and compiled policies retain values
rather than configuration pointers. Each executor then lowers selected limits
with the captured Tool's authored caps.

JavaScript Tools have separate source and combined-input limits from HTTP
response transforms. They also bound each request and the number of pending
fetch/body-read operations. The cumulative native work and allocation budgets
scale with admitted argument, VM, and HTTP data; all operations share the same
remaining allowances. See the [configuration reference](../../docs/src/content/docs/reference/configuration.md#tool-invocation-limits)
for fields, defaults, and accounting formulas.

The private `invokeTool` path authorizes one captured revision, selects its HTTP
or JavaScript policy, and prepares the matching connection. Both executors retain
the deadline started before preparation. A JavaScript Tool uses the helper
library from that same pinned connector generation and receives exactly one
connection capability; global direct `fetch` is absent.

For example, a customer lookup can call a connector's `customerPath(id)` helper,
read a customer through `context.connections[0].fetch(...)`, and then write an
audit record through the same connection. Both requests keep the captured actor
and connection attribution. Per-call labels still select the configured HTTP
middleware. Changing a Tool or connector after admission does not replace the
captured code or configuration during that call.

Library and Tool initialization run once in the execution VM before outbound
requests are enabled. Readiness failures return bounded `notStarted` results;
execution failures after a possible send preserve `mayHaveExecuted`. Caller
cancellation returns a Go error, while an authored/operator timeout can return a
safe Tool failure using the original caller context. Compilation and dependency
diagnostics remain internal errors.

This is an internal composition point. Public invocation still requires the
schema-evaluation work bound tracked in [#948](https://github.com/rmorlok/authproxy/issues/948).
