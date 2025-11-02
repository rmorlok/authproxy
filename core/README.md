# Core

This package implements the central business logic of auth proxy for managing connectors and connections. This
package wraps the database and redis to efficiently load fully hydrated models that expose method for interacting with
the system that will handle appropriate logging an event queuing for background notifications and tasks.

Generally speaking, other packages should not take dependencies on core directly, but rather take dependencies on its
`iface` subpackage that exposes the core interfaces as well as some of the data model used by those interfaces.