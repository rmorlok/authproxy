# Rate Limit Resource Schema

This package owns the canonical `authproxy.net/v1alpha1` `RateLimit` resource,
its update patch, and the desired policy types used by runtime enforcement.

`metadata.namespace` is the hard outer boundary. With no `spec.scope`, a rule
applies to that namespace and its descendants. Scope may narrow it to one
`namespaceMatcher`, typed `connectorRef`, or `connectionRef`. A connector
reference always covers every generation of that connector; RateLimit and
Connector have no generation binding. Matchers and resolved references must
remain at or below the owning namespace. Core normalizes namespaced-name
references to stable IDs before storing the spec.

The database retains its flat columns and serializes only `RateLimitSpec` in
the existing `definition` column. API and configuration boundaries use the
complete resource envelope; enforcement consumes `RateLimitSpec` directly and
does not depend on API DTOs. Pagination and dry-run transport types remain in
`internal/schema/api`.
