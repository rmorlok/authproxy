# authproxy_rate_limit

Manages an AuthProxy rate-limit resource. Every field maps to a typed HCL attribute so authors get plan-time validation and field-level diffs — no `jsonencode` required.

## Example Usage

```hcl
resource "authproxy_rate_limit" "team_acme_writes" {
  namespace = "root.acme"
  mode      = "enforce"

  labels = {
    team = "acme"
  }
  annotations = {
    owner = "platform@example.com"
  }

  selector {
    label_selector = "apxy/connector/-/id=salesforce"
    methods        = ["POST", "PATCH", "PUT"]
    request_types  = ["proxy"]

    path_match {
      kind  = "prefix"
      value = "/services/data/"
    }
  }

  bucket {
    dimensions = ["actor", "labels/team"]
  }

  algorithm {
    token_bucket {
      capacity    = 60
      refill_rate = 1.0
    }
  }
}
```

## Argument Reference

- `namespace` - (Required, ForceNew) The namespace the rate limit belongs to.
- `mode` - (Optional) Either `enforce` (default) or `observe`. In `observe` mode the rule evaluates and records matches but never returns a 429 — useful for safe rollout.
- `labels` - (Optional) Map of user labels.
- `annotations` - (Optional) Map of annotations.
- `selector` - (Required block) Match criteria; all clauses ANDed.
  - `label_selector` - (Optional) Kubernetes-style selector evaluated against the per-request label snapshot.
  - `methods` - (Optional) List of HTTP verbs. Empty / omitted = any.
  - `request_types` - (Optional) Request types the rule applies to. Omit to use the default `["proxy", "probe"]`; an empty list is rejected by the server.
  - `path_match` - (Optional block) Match the final upstream URL path.
    - `kind` - One of `prefix`, `glob`, or `regex`.
    - `value` - The path expression interpreted per `kind`.
- `bucket` - (Required block) Projects matched requests into independent counters.
  - `dimensions` - (Optional) Ordered list of dimensions. Reserved names: `actor`, `connection`, `connector`, `connector_version`, `namespace`, `method`. Label values via `labels/<key>`. Empty / omitted = single global bucket per rule.
- `algorithm` - (Required block) Tagged union — **exactly one** of the following must be set. The provider validates this at plan time.
  - `fixed_window` - Fixed-window counter that resets at floor(now/window) boundaries.
    - `window` - HumanDuration (e.g. `1m`, `5m`).
    - `limit` - Maximum requests per window.
  - `sliding_window` - Sliding-window counter.
    - `window` - HumanDuration.
    - `limit` - Maximum requests within the trailing window.
    - `mode` - `log` (exact) or `counter` (approximate).
  - `token_bucket` - Token-bucket rate limit with refill rate.
    - `capacity` - Maximum tokens (burst capacity).
    - `refill_rate` - Tokens added per second; may be fractional.

## Attribute Reference

- `id` - The server-assigned rate-limit ID (e.g. `rl_abc123`).
- `created_at` - Timestamp of creation.
- `updated_at` - Timestamp of last update.

## Import

Rate limits can be imported by ID:

```bash
terraform import authproxy_rate_limit.example rl_abc123
```
