# Rate Limit Resource Schema

This package owns the canonical `authproxy.net/v1alpha1` `RateLimit` resource,
its update patch, and the desired policy types used by runtime enforcement.

`metadata.namespace` is the broad scope: a rule applies to that namespace and
its descendants. `spec.scope` may narrow the rule to one typed `connectorRef`
(optionally a specific connector generation) or one `connectionRef`. Core
normalizes namespaced-name references to stable IDs before storing the spec.

The database retains its flat columns and serializes only `RateLimitSpec` in
the existing `definition` column. API and configuration boundaries use the
complete resource envelope; enforcement consumes `RateLimitSpec` directly and
does not depend on API DTOs. Pagination and dry-run transport types remain in
`internal/schema/api`.
