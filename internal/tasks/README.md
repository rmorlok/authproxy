# Tasks

This package implements the encrypted locator used for safe customer monitoring
of customer-triggered work. `TaskInfo` binds an Asynq task or go-workflows
instance to the actor allowed to poll it and is encrypted before it leaves the
server.

The locator is a private protocol, not the API `Task` resource projection. It
contains only backend identifiers and workflow/task names; it does not embed a
v1alpha1 resource object. The public response contract lives in
`internal/schema/api`. If a future locator embeds an API resource, introduce a
new explicit locator version before storing or issuing it.

In the future this package might evolve into a database-backed system that can
list every task for an actor.
