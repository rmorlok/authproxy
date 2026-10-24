# Tool request rendering

Tools describe operations that run through a particular AuthProxy connection.
An administrator writes a request template; each invocation supplies arguments
such as a contact ID or a new display name. Connection configuration supplies
values such as the provider's API host and the tenant selected during setup.

This package combines those values into strings and structured JSON for request
construction. It copies invocation data, resolves template references, applies
explicit encoding, and limits rendering work. The HTTP execution layer is
responsible for authorization, input-schema validation, final URL/header/body
validation, authentication, and sending the request. Integration with that layer
is separate from this package's rendering API.

## Example: update a CRM contact

Suppose a CRM connection belongs to tenant `acme`. A Tool updates a contact and
asks the provider to return selected fields. The relevant invocation data is:

```json
{
  "cfg": {
    "apiHost": "crm.example.com",
    "tenant": "acme"
  },
  "params": {
    "contactId": "sales/42",
    "fields": "name email",
    "name": "Ada Lovelace",
    "subscribed": false,
    "tags": ["lead", "web"],
    "phone": null
  }
}
```

The administrator's URL template is:

```mustache
https://{{cfg.apiHost}}/v1/tenants/{{path.cfg.tenant}}/contacts/{{path.params.contactId}}?fields={{query.params.fields}}
```

`Compile` parses this string, and `Template.Render` fills it from a captured
context. The result is:

```text
https://crm.example.com/v1/tenants/acme/contacts/sales%2F42?fields=name+email
```

`cfg.apiHost` comes from the connection. The `path` view encodes the slash in
`sales/42` so the contact ID remains one path segment. The `query` view encodes
the space in `name email` as `+`. These are explicit choices made by the template
author; ordinary interpolation does no URL encoding.

The same invocation can render this JSON body with `RenderValue`:

```json
{
  "name": "{{params.name}}",
  "subscribed": {"$value": "params.subscribed"},
  "tags": {"$value": "params.tags"},
  "phone": {"$value": "params.phone"}
}
```

The rendered body contains these values (formatted here for readability):

```json
{
  "name": "Ada Lovelace",
  "phone": null,
  "subscribed": false,
  "tags": ["lead", "web"]
}
```

`$value` preserves the referenced JSON type. Using `"{{params.subscribed}}"`
instead would produce the string `"false"`, rather than the boolean `false` the
CRM expects. String templates inside structured bodies are JSON-quoted by the
renderer, so names containing quotes remain valid JSON strings.

## Calling the package

Server code passes the maps above as `Input.Params` and `Input.Cfg`. Compile the
URL string with `Compile` and the `common.RawJSON` body with `CompileValue` once
per definition revision. Both compilers take the effective `Limits`, such as
`Limits{MaxBytes: 64 * 1024, MaxDepth: 32, MaxSteps: 10000}`; these example values
are illustrative, and callers choose their policy.

Keep the resulting programs and pass them to this helper for each invocation:

```go
package example

import (
    "context"

    "github.com/rmorlok/authproxy/internal/schema/common"
    "github.com/rmorlok/authproxy/internal/tools/render"
)

// renderContactUpdate prepares a URL and JSON body from one invocation snapshot.
func renderContactUpdate(
    ctx context.Context,
    input render.Input,
    urlTemplate *render.Template,
    bodyTemplate *render.ValueTemplate,
    limits render.Limits,
) (string, common.RawJSON, error) {
    data, err := render.NewContext(input, limits)
    if err != nil {
        return "", nil, err
    }
    url, err := urlTemplate.Render(ctx, data)
    if err != nil {
        return "", nil, err
    }
    body, err := bodyTemplate.Render(ctx, data)
    if err != nil {
        return "", nil, err
    }
    return url, body, nil
}
```

The helper returns the URL and body; it performs no network I/O or template
parsing. Both compiled programs can be reused concurrently. Each invocation
captures its own context; independent renders can safely share that context.

## Invocation snapshot

`NewContext` copies `Input` into private `params`, `cfg`, `labels`, `annotations`,
`connection`, and `tool` scopes. The caller supplies validated input and current
connection configuration/metadata. `connection` contains only `id` and
`namespace`. `tool` contains `id`, `revision`, and optional `toolSetId`,
`toolSetGeneration`, and `catalogRevision` provenance. Empty identity strings and
an unset (zero) tool revision are omitted, so a template requiring them fails
strict lookup rather than silently using an empty value.

The snapshot accepts `map[string]any`, `[]any`, strings, booleans, nulls,
`json.Number`, and native numeric primitives. String maps are also accepted for
metadata. Other typed slices, such as `[]string`, must be converted to `[]any`.
Numbers retain their precision; nonfinite numbers and invalid UTF-8 are rejected.
Nested nil maps/slices remain JSON nulls, while absent root maps become empty
objects. Other Go objects, including structs, pointers, functions, byte slices,
and custom marshalers, are rejected. There are no reflected methods, live
connection handles, or injected credentials. Configuration can contain privileged
values, so errors report reference names rather than the values they resolve.

## String templates

`Compile` parses a Tool-specific Mustache template. `Template.Render` evaluates
it against a snapshot. Existing connector/OAuth rendering in `internal/aptmpl`
is unchanged.

| Syntax | Result |
| --- | --- |
| `{{params.name}}` | Scalar text, without HTML escaping. |
| `{{path.params.id}}` | One URL path segment; `/` becomes `%2F`. |
| `{{query.params.search}}` | Query-string component encoding; a space becomes `+`. |
| `{{json.params.value}}` | A complete JSON literal, including quotes for strings. |

An encoding prefix is recognized only before an explicit invocation root:
`params`, `cfg`, `labels`, `annotations`, `connection`, or `tool`. Encoded
references always read that root, even inside a section. For example,
`{{#params}}{{query.text}}{{/params}}` reads an ordinary `params.query.text`
field; `{{query.params.text}}` encodes the root `params.text` field.

Encoding views are resolved on demand, so both `json.params.object` and
`json.params.object.child` work. Path encoding rejects empty values, `.` and `..`
to avoid empty or traversing path segments. Use the query view only in manually
authored query strings; the [structured query/form codec](../httpcodec/README.md)
applies its own encoding.

For a body authored as a raw string template, the `json` view supplies JSON
quoting and preserves scalar types. With the contact data above, this template:

```mustache
{"name": {{json.params.name}}, "subscribed": {{json.params.subscribed}}}
```

renders to:

```json
{"name": "Ada Lovelace", "subscribed": false}
```

The string renderer returns text; validation of the complete JSON body belongs
to request construction.

Ordinary, triple-brace, and ampersand interpolation have identical behavior.
Triple braces apply only with the default `{{` / `}}` delimiters; use ampersand
interpolation with custom delimiters.
Raw arrays/objects are errors; use the JSON view or a typed directive. Numbers
and booleans render as their JSON text, and explicit null renders as `null`.

Sections, inverted sections, dotted lookup, `.` for the current item, comments,
standalone structural lines, and delimiter changes are supported. Section
lookup starts at the innermost scope. A missing descendant of a found object
does not fall back to an outer object. False, null, and empty arrays skip normal
sections; other values, including zero, empty strings, and empty objects, render
once. Arrays iterate over their items.

For example, a CRM request's text note can summarize the contact's tags and
missing phone number:

```mustache
Tags: {{#params.tags}}[{{.}}]{{/params.tags}}{{^params.phone}}; no phone supplied{{/params.phone}}
```

With the same invocation data, this produces
`Tags: [lead][web]; no phone supplied`. `.` refers to each tag while the array
section runs. The inverted phone section runs because `phone` is explicitly null.
Encoding views are rejected in section controls: use `{{#params.subscribed}}`,
not `{{#query.params.subscribed}}`, so conditions inspect the original JSON value.

Every evaluated reference must exist, including section controls. References
inside skipped sections are not evaluated. Partials and inheritance are rejected
at compilation, even inside skipped sections. Interpolated text is never parsed
again as a template.

## Structured JSON

`CompileValue` parses a `common.RawJSON` document and all embedded string
templates once. `ValueTemplate.Render` evaluates that program against a captured
context. `RenderValue` is a convenience function that compiles and renders a
one-off value.

Object keys remain literal. The `$value` form used in the contact body must have
exactly one key, and its value must be a plain dotted reference. It preserves
false, null, numbers, arrays, and objects without interpreting templates or
directives inside the inserted data. It does not apply an encoding view.

`{"$literal": ...}` copies its entire payload without rendering. For example,
`{"$literal":{"$value":"params.name"}}` produces the literal object
`{"$value":"params.name"}`. `$`-prefixed keys are reserved in template source;
unknown directives and directive objects with additional keys fail compilation.
For example, `{"$value":"params.tags","default":[]}` is an authoring error.
Wrap ordinary data containing such keys in `$literal`. Values inserted through
`$value` also remain uninterpreted data. Structured output uses deterministic
key order and owns its bytes independently of the source and context.

## Limits and verification

Even a short template can produce a large body when it repeats input values.
Nested sections can also multiply the work without producing much output.
Callers must supply positive `Limits` from their execution policy. `MaxBytes`
bounds source and rendered output; during snapshot copying it bounds stored key
and scalar data. `MaxDepth` bounds nesting, and `MaxSteps` bounds visited values,
tags, and section iterations. `CompileValue` shares one compilation work budget
across all JSON nodes and embedded templates. Each compiled-body render gets a
fresh work budget shared across its templates and checks final JSON size after
quoting. The one-off `RenderValue` helper shares a work budget across compilation
and rendering. Encoding views can only be interpolated, so encoded JSON always
contributes to the output byte limit. Render methods observe cancellation and
return no partial result on error.
These are local rendering limits; the later invocation layer must also enforce
its request, response, and total execution budgets.

Run `go test -race ./internal/tools/render ./internal/aptmpl` to check the new
renderer and unchanged legacy behavior.
