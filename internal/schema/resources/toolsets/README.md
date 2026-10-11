# ToolSet resource contracts

This package owns the `ToolSet` resource envelope, generation release fields,
connection selectors, explicit templates, OpenAPI and MCP source settings,
imported defaults and permission mappings, patches, and generation selection policy. It is
a contract foundation: management routes, registry/apply integration, publication
transactions, persistence, and reconciliation are implemented in later slices.
Other OpenAPI import settings and source diagnostics are
also deferred. Strict input decoding and the JSON schema reject those fields
until their contracts are implemented. Source validation does not fetch documents,
resolve references, negotiate protocols, or discover or execute tools.

`metadata` identifies the logical `tls_` resource and its addressed generation.
`spec.connectionSelector` is shared across generations. `spec.release` and
`spec.definition` belong to a generation. Generation responses must project the
current logical selector; a generation update must not restore an old selector.
The generation policy rejects selector writes on explicit generation targets
and keeps selector-only logical updates on the selected resource, even when an
editable draft exists. These membership changes do not migrate attached
connections to a different generation.

The release vocabulary mirrors Connectors. Authors may request `draft` or
`primary`; observed status may be `draft`, `primary`, `active`, or `archived`.
Published generations retain `primary` intent after becoming active or archived.
`ValidateFor` checks snapshot shape and desired/observed compatibility, not
publication transitions, uniqueness, or compiled readiness. Stored/response
resources require ID, name, namespace, positive generation, release intent, and
status. Authoring rejects server-owned status. Namespace is required in every
mode so selector scope can be checked before any connections are selected.

`ApplyAPICreateDefaults` clones the input, assigns the supplied ID, generation 1,
a name derived from the ID when absent, default `draft` intent, and the effective
selector namespace. It does not publish, synthesize a selector, or set status.
Configuration release defaults remain the responsibility of later reconciliation.

## Patches and generation policy

`ToolSetPatch` requires `metadata` and `spec` objects; either can be empty.
`spec.connectionSelector` and `spec.definition` replace their entire fields.
Replacing a selector without `namespace` restores the owner-and-descendants
default, rather than retaining the previous scope. Metadata follows the shared
patch contract: supplied label and annotation maps replace existing maps, and
an empty map clears them. `spec.release` merges only `desiredState`; an empty
release object is a no-op. Selector, definition, release, and desired-state
fields cannot be explicitly null. `SchemaIDToolSetPatch` identifies the embedded
patch schema.

`ApplyTo` validates the patch and complete merged authored fields, preserves
immutable identity and server-owned status, and returns a detached candidate.
It does not perform publication or enforce generation editability. A candidate
can request `primary` while retaining observed `draft` status until publication
succeeds; snapshot validation is not a substitute for transition handling.

`GenerationPolicy` supplies pure hooks for later registry/apply integration.
Observed release status determines whether a generation is editable, published,
or historical. Definition changes and explicit release intent select an
existing draft or the newest generation as a source for a new draft. Metadata
and selector changes alone remain on the selected resource. Finalization
preserves publication intent and includes the selected definition when needed
to distinguish publishing a draft from an already-satisfied primary request.
Explicit generation targets reject selector writes and permit mutations only
on drafts; an empty patch on a published or historical generation is allowed.
Callers remain responsible for routing, authorization, persistence, and atomic
publication. These hooks do not create drafts or migrate bindings themselves.

## Connection selection

`connectionSelector` and its `matchLabels` object are required. Explicit `{}`
means every eligible connection in scope; omission or null is invalid. Omitted
or empty `namespace` defaults to `<toolset namespace>.**`, including the owner
namespace and descendants. Explicit scopes may select the owner, a descendant,
or a subtree beneath the owner. Ancestor and sibling scopes are rejected.

`Compile` returns the validated namespace matcher and a deterministic label
selector string for the existing matcher/query layer. Exact matches are ANDed;
matching an empty value still requires the key to exist. System and inherited
`apxy/` labels can be selected, though they cannot be authored on templates.
Inherited-label propagation is eventual, so reconciliation must recheck current
membership before activation. Selecting a connection does not grant permission
to invoke its generated Tools. `matchExpressions` and set operators are not
supported. JSON/YAML decoding rejects null label values instead of silently
converting them to empty strings, including through YAML aliases and merges.

## Explicit inventories

Use `spec.definition.source.explicit.tools` for the complete template inventory.
The array is required; explicit `[]` deliberately supplies an empty inventory.
Every template requires a nonblank exact `key` and a complete `tools.ToolDefinition`
in `spec`. Keys must be unique within the ToolSet, are never normalized, and need
not be valid display names. Changing a key changes generated identity. Separate
keys may use the same display-name stem because generated names include stable
identity. Templates omit `connectionRef`; installation supplies the bound
connection, namespace, generated identity, revision, and ToolSet ownership.

Optional template `metadata` permits only a name stem, labels, and annotations.
IDs, namespaces, generations, timestamps, and ownership are not authorable.
Each template explicitly declares its verbs and HTTP or JavaScript executor.
Definition validation checks native schemas and authored shape; publication will
compile code/templates before installing an inventory.

## Imported tool defaults

Use `spec.definition.defaults` to supply labels, annotations, and execution
limits for every tool imported from OpenAPI or MCP:

```yaml
source:
  mcp:
    endpoint: "https://{{cfg.apiHost}}/mcp"
    transport: streamableHttp
defaults:
  labels:
    capability: calendar
  annotations:
    owner: platform
  limits:
    timeoutMillis: 30000
    maxRequests: 10
```

Labels and annotations merge by exact key. The administrator's defaults win
when a provider supplies the same key; other permitted provider metadata stays
in place. For example, `owner: platform` replaces a provider's `owner: vendor`,
but leaves its unrelated annotation keys intact. Omitted keys and empty maps
add nothing and do not delete provider metadata. Reserved `apxy/` labels cannot
be supplied. Generated identity, connection binding, and system provenance
remain controller-owned. The importer must validate the final merged metadata,
including annotation size limits.

`limits` uses the same positive execution limits as an individual Tool. In this
example, each generated tool can run for at most 30 seconds and make at most
10 requests, subject to operator ceilings. These are per-tool caps, not a budget
shared by the ToolSet. Omitted limits inherit runtime policy; provider metadata
cannot loosen administrator limits. An empty `limits: {}` adds no caps.

Defaults belong to the published generation. Changing them follows the normal
draft and publication process; existing bindings receive them when migrated to
that generation. Replacing `spec.definition` also replaces its defaults, so
omitting them from a replacement removes the previous authored defaults.
Explicit sources reject `defaults`, even `{}`: their templates declare their own
metadata and limits. Defaults cannot change names, schemas, executors, source
keys, permission verbs, or connection references.

This package validates and preserves the settings. Applying them to provider
content will be implemented with the importers. Hint provenance and source
diagnostics remain separate contracts.

## OpenAPI sources

Exactly one of `spec.definition.source.explicit`, `source.openapi`, or `source.mcp`
is required. An OpenAPI source describes the document to snapshot for a generation:

```yaml
source:
  openapi:
    document:
      url: https://api.example.com/openapi.json
      fetchConnectionRef:
        apiVersion: authproxy.net/v1alpha1
        kind: Connection
        namespace: root.documents
        name: spec-reader
    operations:
      includeOperationIds: [listCalendars, createEvent]
    server:
      url: "https://{{cfg.apiHost}}"
permissionMappings:
  - match:
      sourceKeys: [listCalendars]
    addVerbs: ["tool:calendar.list"]
```

For an inline document, place the OpenAPI object directly under `document.inline`:

```yaml
source:
  openapi:
    document:
      inline:
        openapi: "3.1.0"
        info:
          title: Calendar API
          version: "1.0.0"
        paths:
          /calendars:
            get:
              operationId: listCalendars
              responses:
                "200":
                  description: The available calendars.
                  content:
                    application/json:
                      schema:
                        type: array
                        items:
                          type: object
                          required: [id, name]
                          properties:
                            id: {type: string}
                            name: {type: string}
    server:
      url: "https://{{cfg.apiHost}}"
permissionMappings:
  - match:
      sourceKeys: [listCalendars]
    addVerbs: ["tool:calendar.list"]
```

`document` requires exactly one of `inline` and `url`. Inline content is a JSON
object, also authorable as a YAML mapping. It remains opaque provider data:
extensions, examples, security requirements, and `$ref` fields are retained.
This layer checks object shape without validating an OpenAPI version, compiling
provider schemas, or resolving references. Raw text strings are not accepted
as inline documents.

The acquisition URL is a literal absolute HTTP(S) URL, optionally including a
query, with no user information, fragment, or template placeholders. Go validation
parses the URL. `fetchConnectionRef` is allowed only with a URL and identifies a
Connection by ID or namespaced name without a generation. This connection may
be in a different namespace from the ToolSet and its installation connections.
The acquisition service must check administrative authority before using it and
keep its credentials out of snapshots and unrelated reference requests.

`operations.includeOperationIds` and `excludeOperationIds` select exact document
`operationId` values. Omitted inclusion is unrestricted; supplied inclusion must
be nonempty. Exclusions take precedence, and an empty exclusion list is valid.
Blank/duplicate IDs and null fields or elements are rejected. These filters do
not treat method/path fallback source keys as operation IDs. The importer will
check document identity, unsupported features, and actual operation membership.

### Stable operation identities

OpenAPI identifies an operation by its HTTP method and path. It can also supply
an `operationId`, such as `listCalendars`. AuthProxy uses that ID as the generated
tool's source key, or a deterministic method/path key when the ID is absent.
The source key contributes to the tool's identity and canonical permission verb.
Changing it creates a different generated tool.

Use `sourceKeyOverrides` to keep the same source key after a provider renames
an operation. For example, if `GET /calendars` changes its `operationId` from
`listCalendars` to `listCalendarsV2`:

```yaml
# Under source.openapi, alongside document:
operations:
  includeOperationIds: [listCalendarsV2]
sourceKeyOverrides:
  - method: GET
    path: /calendars
    sourceKey: listCalendars
```

The filter uses the new provider ID, while the override retains the existing
generated identity and verb. Overrides do not select operations or change how
the include/exclude filters match. The same mechanism assigns a stable key to
an operation that has no `operationId`. Permission mappings match the resulting
source key, so they can continue referring to `listCalendars` in this example.

Each override has an exact `method`, `path`, and nonblank `sourceKey`. Use the
method's wire spelling: `GET` for OpenAPI's `get` field. For OpenAPI 3.2
`additionalOperations`, use the exact method key, preserving its case. Paths
must start with `/` and retain their parameter names, such as `/calendars/{id}`.
No method uppercasing, path cleaning, percent-decoding, or template substitution
is performed. See the [OpenAPI Path Item rules](https://spec.openapis.org/oas/v3.2.1.html#path-item-object).

Duplicate method/path targets and duplicate override source keys are rejected.
The importer must also check that each override identifies an operation and
that the final keys do not collide with other operations' IDs or fallback keys.
Overrides do not repair duplicate `operationId` values in an invalid document.
Omitting the list or supplying `sourceKeyOverrides: []` applies no overrides;
null fields, null entries, and unknown fields are invalid.

### Unsupported operations

The importer will default to blocking publication when a selected operation
uses a feature it cannot compile. A broad import can explicitly opt into
skipping unsupported operations and retaining their diagnostics:

```yaml
# Under source.openapi, with no includeOperationIds filter:
unsupportedOperations: skip
```

Omission and `unsupportedOperations: error` keep the blocking policy. `skip`
does not allow an explicitly included unsupported operation to disappear:
that operation still blocks publication until repaired or excluded. Exclusions
take precedence over inclusion. Setting `skip` with an inclusion list is valid
configuration, but does not relax this requirement. A source-key override alone
does not count as inclusion because it only changes identity.

Skipping applies to unsupported operation features, not invalid documents,
identity conflicts, failed acquisition or reference resolution, or ambiguous
security requirements. The importer must produce an inspectable report for
skipped operations. This package validates and preserves the configuration;
operation matching, identity derivation, diagnostics, and publication decisions
remain importer work.

### Server selection and overrides

OpenAPI 3 describes an HTTP API, including its endpoints and the base URLs where
they are available. A provider can list several base URLs under `servers` and
use placeholders such as `{tenant}` in those URLs. For example, this fragment
of a provider's OpenAPI document describes production and sandbox servers:

```yaml
servers:
  - url: https://{tenant}.{region}.example.com/v1
    description: Production
    variables:
      tenant:
        default: demo
      region:
        default: us
        enum: [us, eu]
  - url: https://sandbox.example.com/v1
    description: Sandbox
```

The provider's `variables` section defines the placeholders. Here, `tenant`
defaults to `demo`; `region` defaults to `us` and allows `us` or `eu`. These are
[OpenAPI server variables](https://spec.openapis.org/oas/v3.1.1.html#server-variable-object).

AuthProxy's `OpenAPIServerConfig` chooses the base URL for the generated tools.
It lives at `source.openapi.server`. The separate `document.url` identifies
where to download the specification.

#### Choose a document server

Use `index` to select a server from the provider's list, starting at zero.
Use `variables` to supply values for that server's placeholders:

```yaml
# Under source.openapi:
server:
  index: 0
  variables:
    tenant: acme
    region: eu
```

This selects the production server and produces the base URL
`https://acme.eu.example.com/v1`. A generated tool for `GET /calendars` would
call `https://acme.eu.example.com/v1/calendars`. Leaving out `region` in this
example would use its default, `us`. Using `index: 1` with no variables would
select the sandbox.

`index` is an AuthProxy setting for choosing an entry in the OpenAPI list.
The supplied variable values are fixed for the ToolSet generation: every
connection using that generation gets the same `acme` tenant and `eu` region.

#### Use connection configuration

When the tenant differs by connection, override the base URL with an AuthProxy
template instead:

```yaml
# Under source.openapi:
server:
  url: "https://{{cfg.tenant}}.eu.example.com/v1"
```

Here, `cfg.tenant` comes from the bound connection's stored configuration, such
as a tenant collected during connection setup. A connection with tenant `acme`
would use `https://acme.eu.example.com/v1` as its base URL; one with tenant
`globex` would use `https://globex.eu.example.com/v1`.

**Variable bindings do not support AuthProxy templates in the current contract.**
Putting `tenant: "{{cfg.tenant}}"` inside `variables` keeps that text as a literal
value. Only the `url` form supports runtime connection configuration. Template
support inside variable bindings would require a later change.

#### Selection rules

- Supply exactly one of `url` or `index`. `variables` requires `index`, including
  `index: 0` for the first server. Omitting `server` keeps the importer's default
  selection and the document's variable defaults; `server: {}` is invalid.
- OpenAPI lets an endpoint declare its own server list. An operation's list
  overrides its path's list, which overrides the document-wide list. AuthProxy
  applies `index` to that operation's list, without merging lists or falling
  back to a parent if the index is out of range.
- Bindings use exact, nonblank variable names and literal string values. Empty
  strings are allowed; empty `variables: {}` supplies no overrides. Null fields
  or values, unknown fields, and negative indices are rejected.
- The importer will check server bounds, declared variables, allowed values,
  and relative URL resolution. Swagger 2 has no `servers` list, so indexed
  selection does not apply; omission or a URL override remains available.

This slice defines and validates the configuration. Server resolution, variable
substitution, and URL template execution will be implemented in the importer
and execution layers.

### Security scheme mappings

OpenAPI names authentication methods in `components.securitySchemes`
(`securityDefinitions` in Swagger 2). An operation's `security` field says which
methods it requires. An operation can override the document-wide requirements;
`security: []` removes them. Each object in the array is an alternative, and all
schemes within that object are required together. For example:

```yaml
# Inside the provider's OpenAPI document:
security:
  - calendarOAuth: [calendars.read]
  - apiKey: []
```

This allows OAuth with the `calendars.read` scope **or** an API key. Putting both
names in one object would require **both** methods. An empty object (`{}`) is an
alternative that allows anonymous access. See the
[OpenAPI security requirement rules](https://spec.openapis.org/oas/v3.2.1.html#security-requirement-object)
and [operation overrides](https://spec.openapis.org/oas/v3.2.1.html#operation-object).

AuthProxy needs an explicit mapping from those provider identifiers to the
connection that will receive the tools:

```yaml
# Under source.openapi, alongside document:
security:
  connectionSchemes: [calendarOAuth]
```

This tells the importer to satisfy `calendarOAuth` through the bound connection's
existing authentication. It does not select the connection that downloads the
OpenAPI document, add credentials, or change how the proxy injects them. There
is no second execution connection: every tool remains bound to one connection.

For the provider example above, the importer must verify that this connection
can satisfy the OAuth alternative, including `calendars.read`. Listing `apiKey`
as well would map that scheme to the same connection; it would not supply another
credential or make an unsupported combination work. Mapping membership alone
does not prove compatibility. The importer and binding layer must check the
scheme type, credential placement, and required scopes or roles, and reject
requirements they cannot support. They must satisfy a complete alternative,
not combine parts of different alternatives. List order does not choose one.

`connectionSchemes` contains unique, nonblank, exact Security Requirement keys.
Case and spelling are preserved, including URI references supported by OpenAPI
3.2; this field does not use patterns or URI normalization. Resolution of those
identifiers belongs to the importer. Null fields or list entries and unknown
configuration fields are rejected.

Omitting `security`, supplying `security: {}`, or using `connectionSchemes: []`
maps no schemes. None of these forms disables the provider's requirements or
suppresses the connection's normal authentication. Required scopes and roles
come from the provider document, so they are not repeated in this configuration.
AuthProxy's `permissionMappings` separately controls tool verb aliases; security
scheme mappings do not grant an actor permission to invoke a tool.

This package validates and preserves the mapping configuration. Document
resolution, alternative selection, compatibility checks, and execution are
implemented in later importer and binding slices.

### Reference bundles

OpenAPI uses `$ref` to reuse schemas and other definitions. A fragment-only
reference such as `#/components/schemas/Item` selects content within the current
reference resource. A reference such as `schemas/item.json` identifies another
document; `schemas/item.json#/$defs/Item` also selects a fragment within it.
Relative references need a base URI to identify their target.

`OpenAPIReferenceConfig` lives at `source.openapi.references`. It supplies an
optional `baseUri` for the entry document and an offline `bundle` of documents
keyed by their absolute URI identities:

```yaml
source:
  openapi:
    document:
      inline:
        openapi: "3.1.0"
        info:
          title: Item API
          version: "1.0.0"
        servers:
          - url: https://api.example.test
        paths:
          /item:
            get:
              operationId: getItem
              responses:
                "200":
                  description: The requested item.
                  content:
                    application/json:
                      schema:
                        $ref: schemas/item.json
    references:
      baseUri: https://example.test/api/openapi.yaml
      bundle:
        https://example.test/api/schemas/item.json:
          type: object
          required: [id]
          properties:
            id: {type: string}
            name: {type: string}
```

Here, resolving `schemas/item.json` against the entry document's `baseUri`
produces `https://example.test/api/schemas/item.json`, the bundle key. The
importer will obtain that document from the supplied bundle. The key becomes
that document's retrieval identity for resolving its own relative references.

`baseUri` supplies the entry document's intended retrieval context. If omitted,
a document fetched through `document.url` gets its context from its retrieval
URL. It does not override the standard precedence of OpenAPI 3.2 `$self` or
JSON Schema `$id`. Those content-defined identities, fragments, and reference
resolution are importer responsibilities. See the
[OpenAPI reference-resolution rules](https://spec.openapis.org/oas/v3.2.1.html#relative-references-in-api-description-uris).

The base URI and bundle keys must be literal absolute URIs without fragments,
user information, whitespace, or template placeholders. Bundle keys cannot be
relative; `baseUri` provides context for references inside the document, not for
those keys. URI identities are not fetch instructions: a `urn:` or `file:` identity
can name offline content without accessing a network or filesystem. Credentials
from `document.fetchConnectionRef` are never inherited by these identities.

Bundle values remain opaque JSON, including boolean schemas, arrays, nulls,
and nested provider extensions. Valid JSON alone does not promise a supported
reference target. The importer will check document versions, target types,
`$id` and other identity collisions, and complete reference resolution. The
authoring schema and Go validation perform no I/O or provider-schema validation.
Unknown configuration fields, null configuration fields, and duplicate bundle
keys are rejected; a null value inside a bundled document remains opaque data.
Duplicate-key detection compares decoded JSON strings exactly, including case.
URI schemes and hosts are case-insensitive, while paths and queries can be
case-sensitive ([RFC 3986](https://www.rfc-editor.org/rfc/rfc3986.html#section-6.2.2.1)).
The authoring codec preserves all URI spellings. Detecting equivalent URI
identities, such as host-case variants, belongs to the later importer.
Omitting `references`, supplying `references: {}`, or supplying `bundle: {}`
adds no bundled documents. Bundle configuration alone does not permit external
fetching.

### External reference acquisition

An OpenAPI document can refer to a schema hosted at another URL. AuthProxy
requires each external document to be listed in `references.fetch` before the
importer can retrieve it. Each entry names one document, rather than allowing
access to an entire host or every URL found in the specification.

For example, if the root document contains
`$ref: https://api.example.test/schemas/item.json#/$defs/Item`, configure the
document URL without the fragment. The root document and reference can use
the same connection:

```yaml
# Under source.openapi:
document:
  url: https://api.example.test/openapi.yaml
  fetchConnectionRef:
    apiVersion: authproxy.net/v1alpha1
    kind: Connection
    namespace: root.documents
    name: specification-reader
references:
  fetch:
    - url: https://api.example.test/schemas/item.json
      fetchConnectionRef:
        apiVersion: authproxy.net/v1alpha1
        kind: Connection
        namespace: root.documents
        name: specification-reader
    - url: https://public.example.test/common.json
```

Both the root document and `item.json` use `specification-reader`. A separate
connection is useful when a reference needs different credentials, such
as a shared schema service with its own authentication. A public reference
can omit `fetchConnectionRef`, as `common.json` does here.

Each authenticated entry explicitly names its connection, even when reusing
the root document's connection. Omitting `fetchConnectionRef` means no
credentials; it never inherits them from another fetch. This prevents root
document credentials from being sent to an unrelated reference URL by default.
The importer resolves and authorizes each acquisition connection separately
from the target connections that will receive tools.

The importer will snapshot the fetched documents along with supplied bundle
content. If `item.json` refers to `common.json` at the public URL above, the
second entry permits that fetch. Any further external document must also be
listed or supplied in `bundle`. Relative references use their containing
document's retrieval context, subject to content-defined identities such as
`$id`; `baseUri` does not turn relative fetch entries into absolute URLs.

- URLs must be literal absolute HTTP(S) addresses without user information,
  fragments, or AuthProxy templates. Configure the final URL: this initial
  acquisition contract does not permit redirects.
- Each URL may occur once in `fetch` and cannot also be a `bundle` key. Go
  validation checks these exact-string conflicts; the importer will detect
  equivalent URI identities and content-defined aliases.
- Omitting `fetch` or supplying `fetch: []` permits no external reference
  fetches. Null fields, null entries, and unknown configuration fields are
  invalid. Connection references have no generation.
- Fetch instructions belong to the ToolSet generation. They apply during
  import only; validation and tool invocation use no network reference
  fetching, and invocation uses the published snapshot.

This package defines and validates acquisition instructions; it performs no
fetching. Actual acquisition, reference resolution, immutable snapshots,
refresh, server resolution, and operation compilation are future work.

## MCP sources and imported permission mappings

MCP settings belong to a published generation; the live catalog will be
discovered separately through each bound connection. The endpoint template can
read that connection's `cfg`, and `transport` must be `streamableHttp`.
URL/template compilation, protocol negotiation, refresh scheduling, and
connection-scoped discovery are adapter/controller responsibilities.

```yaml
source:
  mcp:
    endpoint: "https://{{cfg.apiHost}}/mcp"
    transport: streamableHttp
    refreshInterval: 5m
    tools:
      excludeNames: [admin_reset]
permissionMappings:
  - match:
      sourceKeys: [list_calendars]
    addVerbs: ["tool:calendar.list"]
```

`refreshInterval` is optional positive Go duration text, such as `5m` or `1.5s`;
the authored string is preserved across JSON/YAML round trips. Omission leaves
the interval to runtime policy. Go validation checks positivity and overflow in
addition to the syntax described by the JSON schema.

`tools.includeNames` and `tools.excludeNames` match exact upstream names without
normalization or pattern expansion. Omitted inclusion means all names; an
explicit empty inclusion list is rejected to avoid silently broadening selection.
Exclusions take precedence over inclusions. An empty exclusion list is allowed.
Null fields/lists/elements, blank names, and duplicate names within a list are
rejected. `tools: {}` imposes no name restrictions.

`permissionMappings` is optional for OpenAPI and MCP sources; explicit templates
declare their own verbs. Each rule requires a `match` with
at least one `sourceKeys` or `sourceKeyPatterns` entry and a nonempty `addVerbs`
list. Keys remain exact, unnormalized identities. Patterns use Go regular
expressions with full-string semantics (`\A(?:pattern)\z`), including across
slash characters; exact keys and patterns combine with OR. Broad patterns
deliberately cover future matching tools. Blank/duplicate entries and invalid
patterns are rejected; aliases also reject surrounding whitespace.

The importers will add the union of every matching rule's aliases to the Tool's
canonical verb. Mappings never replace that verb, infer aliases from provider
hints, or grant permission by themselves. Alias resolution and canonical verb
generation are outside this schema slice. Validation does not require current
catalog keys to exist, since each connection's catalog can differ and change.
Definition replacements, including filter/mapping changes, remain generation
changes under the existing patch policy.

Use `util.DecodeJSONStrict` / `util.DecodeYAMLStrict` at input boundaries followed
by resource lifecycle validation. `SchemaIDToolSets` identifies the embedded
offline JSON schema. Cross-field namespace boundaries, duplicate source keys,
and lifecycle policies are also enforced in Go. `Clone` methods return detached
snapshots without serialization, preserving raw bytes and invalid/missing values
so preparing a candidate cannot mutate a prior generation or hide diagnostics.
