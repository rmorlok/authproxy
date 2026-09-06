# Workflows

This package adapts go-workflows runtime, client, worker, and diagnostic APIs
for AuthProxy. The persisted workflow history and argument encoding are private
go-workflows protocols, not AuthProxy API resources.

Durable workflow registrations include a version suffix such as `.v1`, and
their Go input types carry the same version. Current workflow inputs contain
scalar AuthProxy IDs and operation parameters; none embeds a v1alpha1 resource
object. The task/queue/workflow API migration therefore leaves persisted
arguments, retry behavior, resume behavior, and idempotent instance IDs intact.
Any future workflow input that embeds a resource must use a new registration
name and input type rather than changing a persisted definition in place.
