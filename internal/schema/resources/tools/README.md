# Tool resources and authored definitions

This package owns `ToolDefinition`, the reusable definition shared by standalone
Tools and explicit ToolSet templates. It contains descriptions, permission verb
aliases, input/output schemas, behavioral hints, execution limits, and exactly
one `proxyHttp` or `javascript` executor. Definitions are trusted administrator
input; hints do not authorize an operation or prove that it is safe to retry.

`Tool` adds the resource envelope: `metadata` carries a `tol_` identity and
namespace, `spec` combines `connectionRef` with the flat definition fields, and
server-owned `status` carries a positive revision and readiness conditions.
Tools have no `metadata.generation`. Conditions report `observedRevision`;
zero or omission means no revision has been observed yet. Generated Tools carry
`managedBy.toolSetRef` with a canonical `tls_` ID and applied generation, plus
an exact `sourceKey`. These contracts currently describe authored executors,
including explicit templates materialized by a future ToolSet controller.

This is a contract foundation. ToolSet resources, compiler-produced OpenAPI/MCP
plans, management routes, registry/apply support, persistence, and execution are
introduced in later changes. The package does not import API DTOs or register a
usable resource prematurely.

Call `Tool.ValidateFor` with the relevant lifecycle mode. Authored resources
accept a connection ID or namespace/name; stored and response resources require
the resolved ID and status. An explicit connection namespace must equal the
Tool namespace. The service must resolve references and verify the actual
target namespace, including for ID-only references. An explicit ToolSet owner
namespace must be the Tool's namespace or an ancestor. Status is rejected on
authoring writes; neither schema validation nor patch application publishes or
increments a revision.

`ToolPatch` requires `metadata` and `spec` objects, which may be empty. Omitted
spec fields preserve the current value; supplied fields replace the entire
field. Null clears optional output schemas, hints, limits, and executors. Null
is invalid for connection references, descriptions, verbs, and input schemas.
Switching executors requires clearing the old executor and supplying the new
one in the same patch. Metadata follows the shared patch convention: omitted
or null maps retain the current map, while `{}` clears it. `ApplyTo` validates
the complete resulting definition and returns a detached candidate, preserving
the original resource and its server status. Identity and namespace stay
immutable; standalone connection references may change within that namespace.
Direct updates to generated Tools fail with the owning ToolSet identity.

Use the repository's strict JSON/YAML decoders at input boundaries, then call
`ToolDefinition.Validate`. The embedded JSON schema describes the authored wire
shape. `SchemaIDToolResource` and `SchemaIDToolPatch` identify the separate
resource and partial-update schemas in `schema-resource.json`. Go validation
also compiles the native schemas and checks constraints
such as conflicting HTTP error rules. Unknown authored fields, including
`openapiOperation` and `mcpCall`, are rejected by strict decoding and the schema.
The schema's open `ToolDefinitionFields` definition supports flat spec
composition; `ToolDefinition` and the schema root close unknown fields.

Input schemas explicitly declare top-level `type: object` (or `[object]`).
Output schemas may be objects or boolean schemas. Native schemas use JSON Schema
2020-12; local references are resolved without loading network or filesystem
resources. Keyword-looking keys inside defaults, examples, or constants remain
data. Compilation checks a definition; it does not apply defaults, mutate
invocation arguments, or execute JavaScript.
Each schema is limited to 256 KiB and 64 nested JSON containers. Dialect checks
apply at compiled schema locations; unused definitions are checked when a
reference makes them part of the compiled schema.

`proxyHttp` permits no body or one of `bodyJson`, `bodyTemplate`, `bodyRaw`,
`form`, and `multipart`. `bodyJson: null` is a present JSON body and stays
distinct from omission in both JSON and YAML. Structured values preserve their
JSON types and may use the reserved single-key `$value` and `$literal` forms.
Raw binary values are base64, never filesystem paths. Media types for raw
bodies belong in the headers; `bodyTemplate` declares its media type explicitly.
Multipart parts contain either `text` or `bodyRaw` with optional filename and
media type.

HTTP error rules select exact `statuses`, one `statusClass` (1 through 5), or
`default: true`. Repeated selectors at the same priority conflict. Exact status,
class, and default rules may overlap, with that precedence. Response transforms
and request templates are only stored here; later compilers validate their
syntax and the executor checks rendered URLs, headers, and encoded values.

Optional execution limits use exact positive integers: `timeoutMillis` is in
milliseconds; `maxInputBytes`, `maxOutputBytes`, `maxResponseBytes`, and
`maxEncodedResultBytes` are byte counts; `maxRequests` and `maxStackDepth` are
counts. Omission inherits runtime policy. Runtime defaults, operator ceilings,
the shared aggregate content budget, and enforcement remain execution-layer
responsibilities; these contracts do not make any limit unlimited.
