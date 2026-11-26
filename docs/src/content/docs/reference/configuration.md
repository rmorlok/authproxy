---
title: Configuration reference
---

AuthProxy configuration is YAML validated against the canonical JSON Schema at
[`internal/schema/config/schema.json`](https://github.com/rmorlok/authproxy/blob/main/internal/schema/config/schema.json).
The development configuration demonstrates the full server shape at
[`dev_config/default.yaml`](https://github.com/rmorlok/authproxy/blob/main/dev_config/default.yaml).

## Major blocks

| Block | Purpose |
|---|---|
| `public`, `api`, `adminApi`, `worker` | Enabled services, ports, TLS, UI, and health behavior |
| `hostApplication`, `marketplace` | Browser login handoff and Marketplace URL |
| `systemAuth` | JWT, actors, global encryption key, and DEK policy |
| `database`, `redis` | Primary database and distributed state |
| `appMetrics` | Request-event, resource-metric, and optional blob storage |
| `tasks` | Task retention and worker behavior |
| `tools` | Tool invocation time, request, data, and processing limits |
| `telemetry` | OTLP exporter, signals, sampling, and label projection |

Fields can use AuthProxy value sources such as direct development values,
environment variables, and file paths. Never put production credentials or
key material directly in a committed YAML file.

## Workflow retention

The worker removes finished workflow instances and their history once a day.
Set `tasks.workflowRetention` to control how long they are kept after they
finish. The default is 30 days:

```yaml
tasks:
  workflowRetention: 720h
```

Instances that are still running are never removed. Set `0s` to disable the
cleanup and keep finished instances indefinitely. Use hours rather than days in
YAML files.

Once an instance is removed, it no longer appears in workflow monitoring, and
polling its task returns `404`.

## Connector manifests

Define connectors as `authproxy.net/v1alpha1` `Connector` resources in YAML or
JSON manifests, and manage them through
[`ap apply`](/development/cli/#apply-resource-manifests). Store manifests in
separate files or combine them in a multi-document YAML file. Server configuration
contains service and infrastructure settings; connector manifests are separate
inputs to apply.

Give each new connector a `metadata.name` and `metadata.namespace`. Include
Namespace manifests in the same apply batch when those namespaces do not exist.
See [resource manifests](/reference/resources/) for the manifest structure.

Use `ap apply -f resources.yaml` after deployment. For local development, use
[`serve --apply`](/development/cli/#apply-resources-on-server-startup) to apply
a file or directory once the API is available.

Apply identifies an existing connector by its namespace/name or `metadata.id`.
An explicit ID must already exist; omit IDs when creating new resources. For
Connector manifests, omit `metadata.generation`; apply creates a new generation
when needed. Use ConnectorGeneration manifests for explicit generation lifecycle
operations.

Removing a manifest does not delete its resource. Use explicit resource deletion
or the opt-in pruning options of `ap apply` when deletion is intended.

## Configured actor namespaces

Inline configured actors are complete `authproxy.net/v1alpha1` `Actor`
resources and set their namespace in `metadata.namespace`. For actors
discovered from public-key directories, key each source by the namespace that
owns its actors:

```yaml
systemAuth:
  actors:
    root:
      keysPath: /etc/authproxy/keys/actors/root
      permissions:
        - namespace: root.**
          resources: ["*"]
          verbs: ["*"]
    root.smoke:
      keysPath: /etc/authproxy/keys/actors/smoke
      permissions:
        - namespace: root.smoke
          resources: [connectors]
          verbs: [list]
        - namespace: root.smoke.{{external_id}}
          resources: [connections]
          verbs: [create, get, proxy]
    syncCronSchedule: "* * * * *"
```

Every `.pub` file in a source directory creates an actor in that source's
namespace. For example,
`/etc/authproxy/keys/actors/smoke/smoke-user.pub` creates `smoke-user` in
`root.smoke`. Permissions are source-specific, and permission namespaces can
use actor templates such as `{{external_id}}`. Development migration creates
each configured actor namespace and its missing parents before synchronization.
Directory sources are keyed by namespace, as shown above.

## Tool invocation limits

Set `tools.invocation` to control the server's limits for each Tool call. For
example, this allows 30 seconds, up to 10 logical requests, and 512 KiB of
arguments:

```yaml
tools:
  invocation:
    timeout: 30s
    maxRequests: 10
    maxInputBytes: 512kib
    maxResponseBytes: 8mib
    maxOutputBytes: 8mib
    maxEncodedResultBytes: 16mib
```

Omitted fields use the defaults below. Explicit values must be positive; zero
does not mean unlimited. Sizes use strings such as `512kib` or `8mib` (binary
units); `kb` and `mb` use decimal units. Per-Tool limits can lower the server's
limits but cannot raise them. For example, a Tool with `timeoutMillis: 5000`
gets five seconds under the configuration above.

These settings govern the internal HTTP and JavaScript Tool invocation paths. There
is no public invocation endpoint yet; bounded schema evaluation remains a
prerequisite. The declarative HTTP executor makes one logical request. Proxy
credential recovery and redirects may make additional HTTP attempts under the
existing proxy policy. JavaScript Tools can make multiple requests through their
single captured connection; preparation and execution share the timeout.
JavaScript-specific settings below do not change existing connector scripts or
HTTP response transforms.

Standalone JavaScript Tool publication uses a separate fixed validation profile:
256 KiB of combined Tool and connector-helper source and a five-second allowance
for helper preparation and initialization. It does not call `execute` or provide
HTTP capabilities. Changing `tools.invocation` does not change this publication
profile.

| Field | Default | What it limits |
|---|---|---|
| `timeout` | `60s` | Preparation, compilation, and execution after authorization; response transforms share the remaining time |
| `maxRequests` | `20` | Logical connection-client calls |
| `maxRequestBytes` | `1mib` | Each JavaScript request's URL, method, headers, body, and label overrides before dispatch |
| `maxPendingOperations` | `8` | Concurrent JavaScript fetch and body-read operations, including completions waiting for the VM |
| `maxInputBytes` | `1mib` | Raw argument JSON, including whitespace |
| `maxResponseBytes` | `8mib` | Aggregate decoded response bytes |
| `maxOutputBytes` | `8mib` | Result content, structured-output JSON, and transform output; JavaScript also applies this cap to each complete rich result, `ToolError` payload, and console record |
| `maxEncodedResultBytes` | `16mib` | The complete JSON result, including metadata, escaping, and base64 |
| `maxStackDepth` | `256` | JavaScript Tool and response-transform function-call depth |
| `maxJavascriptSourceBytes` | `256kib` | Combined JavaScript Tool and selected connector helper-library source, including whitespace |
| `maxJavascriptInputBytes` | `16mib` | Combined serialized arguments, Tool context, and the captured connection supplied to a JavaScript Tool |
| `maxRenderBytes` | `8mib` | Each copied invocation context, compiled request template, and rendered request |
| `maxTransformInputBytes` | `16mib` | Serialized invocation context and response supplied to a transform |
| `maxResponseStreamBytes` | `8mib` | Each response stream before Tool-host decoding; the HTTP transport may already have decompressed it |
| `maxResponseHeaderBytes` | `64kib` | Each copied response header map, including line overhead |
| `maxResponseSourceBytes` | `256kib` | Combined response-rule and transform source text |
| `maxDepth` | `64` | Nesting of copied, rendered, validated, and transformed data |
| `maxValues` | `1000000` | Values inspected per JSON validation or JavaScript input/output/body conversion |
| `maxRenderSteps` | `1000000` | Work per context copy, template compilation, or render |
| `maxNumberDigits` | `4096` | Mantissa digits plus absolute decimal exponent for JSON validation; cannot exceed 4096 |

The byte allowances are independent. Arguments may be small while connection
configuration or a response makes the transform input large. Raising
`maxResponseBytes` does not raise the stream, transform-input, output, or result
limits. A 16 MiB result allowance fits base64 encoding of 8 MiB of binary content
with room for metadata, but heavily escaped text can need more space.

Response transforms also share cumulative work and allocation budgets across
input preparation, pure utilities, and result conversion. Each budget is derived
from the resolved operator limits as
`16 * (maxTransformInputBytes + maxOutputBytes) + 32 * maxValues`, using byte
counts for the size fields. Raising those settings therefore raises the allowance
for native scans, copies, and metadata processing. Repeated or expensive utility
calls can still exhaust it. A combination too large to fit the runtime's integer
range is rejected when the invocation policy is resolved.

JavaScript Tools use one cumulative work budget and one cumulative allocation
budget across arguments, VM input, HTTP conversions, pure utilities, and results.
Each is derived from the operator's byte and value limits as:

```text
16 * (maxInputBytes + maxJavascriptInputBytes + maxOutputBytes + maxResponseBytes
      + maxRequests * (maxRequestBytes + maxResponseHeaderBytes))
  + 32 * maxValues
```

Request and header processing scale with the request count. Response bytes are
already an aggregate allowance, so they are counted once. Raw argument bytes are
included separately because whitespace is removed before VM input. Raising these
settings increases the native processing allowance; arbitrary repeated work can
still exhaust it. Overflow is rejected before execution. Per-Tool caps lower
their corresponding limits without recomputing this operator budget.

The request and worker limits serve different purposes. For example,
`maxRequests: 10` and `maxPendingOperations: 2` allow ten logical calls over the
whole execution, with at most two fetch or body-read operations pending at once.
Reading a response body uses a worker slot but does not spend another logical
request. Raising the worker limit does not grant more requests or a fresh native
budget. JavaScript source/input limits are independent of response-transform
source/input settings.

Nesting and value counts bound data processing; they do not count JavaScript
instructions or JSON Schema evaluation steps. The timeout interrupts supported
work and HTTP I/O, but cannot preempt synchronous schema-library evaluation.
These limits are not a hard memory quota.


## Kubernetes values

The Helm chart exposes typed values for common deployment settings and merges
`config` as an advanced overlay. Its reference is
[`deploy/charts/authproxy/values.yaml`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/values.yaml)
with validation in
[`values.schema.json`](https://github.com/rmorlok/authproxy/blob/main/deploy/charts/authproxy/values.schema.json).

Prefer typed chart values for database, Redis, service, ingress, Secret, and
storage configuration. Use the free-form overlay only when a server setting has
not yet been promoted into the chart schema.

## Validate changes

Start the server or render the chart in CI to exercise schema validation. For
repository changes, run:

```bash
./scripts/preflight.sh
```

Connector definitions have their own schema and authoring guides; continue
with [connector setup flows](/integration/connector-setup-flow/) and
[connector predicates](/integration/connector-predicates/).
