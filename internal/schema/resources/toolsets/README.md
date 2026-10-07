# Explicit ToolSet resource contracts

This package owns the `ToolSet` resource envelope, generation release fields,
connection selectors, and explicit template definitions. It is a contract
foundation: management routes, registry/apply support, generation patches and
publication, persistence, and reconciliation are implemented in later slices.
OpenAPI/MCP source contracts, filters, defaults, permission mappings, and source
diagnostics are also deferred. Strict input decoding and the JSON schema reject
those fields until their contracts are implemented.

`metadata` identifies the logical `tls_` resource and its addressed generation.
`spec.connectionSelector` is shared across generations. `spec.release` and
`spec.definition` belong to a generation. Generation responses must project the
current logical selector; a generation update must not restore an old selector.
The future update policy will reject selector writes on generation endpoints
and treat selector-only logical updates as membership changes, not migrations.

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

Use `util.DecodeJSONStrict` / `util.DecodeYAMLStrict` at input boundaries followed
by resource lifecycle validation. `SchemaIDToolSets` identifies the embedded
offline JSON schema. Cross-field namespace boundaries, duplicate source keys,
and lifecycle policies are also enforced in Go. `Clone` methods return detached
snapshots without serialization, preserving raw bytes and invalid/missing values
so preparing a candidate cannot mutate a prior generation or hide diagnostics.
