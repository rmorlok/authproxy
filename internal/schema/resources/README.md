# Resource Schemas

This tree contains schema packages for resources managed by AuthProxy in a RESTful style.

Resource packages can depend on `internal/schema/common` and on other resource packages when there is a genuine resource relationship. They must not import `internal/schema/api`; API request and response wrappers compose resources from the outside.
