# Tool request rendering

This package implements the rendering part of [the Tools design](../../../docs/src/content/docs/development/design/tools-and-toolsets.md).
It prepares strings and structured JSON for later request construction. It does
not validate input schemas, authorize an invocation, validate a completed HTTP
request, or send traffic. Those responsibilities belong to later execution
layers.

## Invocation snapshot

`NewContext` copies `Input` into private `params`, `cfg`, `labels`, `annotations`,
`connection`, and `tool` scopes. The caller supplies validated input and current
connection configuration/metadata. `connection` contains only `id` and
`namespace`. `tool` contains `id`, `revision`, and optional `toolSetId`,
`toolSetGeneration`, and `catalogRevision` provenance.

The snapshot accepts JSON maps, arrays, strings, booleans, nulls, `json.Number`,
and native numeric primitives. String maps are also accepted for metadata.
Numbers retain their precision; nonfinite numbers and invalid UTF-8 are rejected.
Nested nil maps/slices remain JSON nulls, while absent root maps become empty
objects. Other Go objects, including structs, pointers, functions, byte slices,
and custom marshalers, are rejected. There are no reflected methods, live
connection handles, or injected credentials. Configuration can contain privileged
values, so errors report reference names rather than the values they resolve.

## String templates

`Compile` parses a Tool-specific Mustache template. `Template.Render` evaluates
it against a snapshot. Compiled templates and snapshots can be reused
concurrently. Existing connector/OAuth rendering in `internal/aptmpl` is unchanged.

| Syntax | Result |
| --- | --- |
| `{{params.name}}` | Scalar text, without HTML escaping. |
| `{{path.params.id}}` | One URL path segment; `/` becomes `%2F`. |
| `{{query.params.search}}` | Query-string component encoding; a space becomes `+`. |
| `{{json.params.value}}` | A complete JSON literal, including quotes for strings. |

The same views work for `cfg` and other scopes. Encoding views are resolved on
demand, so both `json.params.object` and `json.params.object.child` work. Use the
query view only in manually authored query strings; the future structured-query
encoder will apply its own encoding.

Ordinary, triple-brace, and ampersand interpolation have identical behavior.
Raw arrays/objects are errors; use the JSON view or a typed directive. Numbers
and booleans render as their JSON text, and explicit null renders as `null`.

Sections, inverted sections, dotted lookup, `.` for the current item, comments,
standalone structural lines, and delimiter changes are supported. Section
lookup starts at the innermost scope. A missing descendant of a found object
does not fall back to an outer object. False, null, and empty arrays skip normal
sections; other values, including zero, empty strings, and empty objects, render
once. Arrays iterate over their items.

Every evaluated reference must exist, including section controls. References
inside skipped sections are not evaluated. Partials and inheritance are rejected
at compilation, even inside skipped sections. Interpolated text is never parsed
again as a template.

## Structured JSON

`RenderValue` recursively renders string values in a `common.RawJSON` document.
Object keys remain literal. An exact single-key directive preserves the type of
the referenced value:

```json
{
  "description": "Created for {{params.name}}",
  "enabled": {"$value": "params.enabled"},
  "items": {"$value": "params.items"}
}
```

`$value` resolves a plain dotted reference. It preserves false, null, numbers,
arrays, and objects without interpreting templates or directives inside the
inserted data. It does not apply an encoding view.

`{"$literal": ...}` copies its entire payload without rendering. For example,
`{"$literal":{"$value":"params.name"}}` produces the literal object
`{"$value":"params.name"}`. An object containing `$value` or `$literal` alongside
other keys is an ordinary object. Structured output uses deterministic key order
and owns its bytes independently of the source and context.

## Limits and verification

Callers must supply positive `Limits` from their execution policy. `MaxBytes`
bounds source and rendered output; during snapshot copying it bounds stored key
and scalar data. `MaxDepth` bounds nesting, and `MaxSteps` bounds visited values,
tags, and section iterations. A structured body shares its rendering work budget
across all embedded templates and checks final JSON size after quoting. Both
render entry points observe cancellation and return no partial result on error.
These are local rendering limits; the later invocation layer must also enforce
its request, response, and total execution budgets.

Run `go test -race ./internal/tools/render ./internal/aptmpl` to check the new
renderer and unchanged legacy behavior.
