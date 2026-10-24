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

Server code passes the maps above as `Input.Params` and `Input.Cfg`. The following
helper accepts the URL template as a string and the body template as
`common.RawJSON`, then returns their rendered values for HTTP request construction:

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
    urlSource string,
    bodySource common.RawJSON,
) (string, common.RawJSON, error) {
    // Illustrative values; callers choose limits from their execution policy.
    limits := render.Limits{MaxBytes: 64 * 1024, MaxDepth: 32, MaxSteps: 10000}
    data, err := render.NewContext(input, limits)
    if err != nil {
        return "", nil, err
    }
    template, err := render.Compile(urlSource, limits)
    if err != nil {
        return "", nil, err
    }
    url, err := template.Render(ctx, data)
    if err != nil {
        return "", nil, err
    }
    body, err := render.RenderValue(ctx, bodySource, data, limits)
    if err != nil {
        return "", nil, err
    }
    return url, body, nil
}
```

The helper returns the URL and body; it performs no network I/O. A compiled
`Template` can be reused across invocations. Each invocation captures its own
context; independent renders can safely share that context.

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
it against a snapshot. Existing connector/OAuth rendering in `internal/aptmpl`
is unchanged.

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

Every evaluated reference must exist, including section controls. References
inside skipped sections are not evaluated. Partials and inheritance are rejected
at compilation, even inside skipped sections. Interpolated text is never parsed
again as a template.

## Structured JSON

`RenderValue` recursively renders string values in a `common.RawJSON` document.
Object keys remain literal. The `$value` form used in the contact body must have
exactly one key, and its value must be a plain dotted reference. It preserves
false, null, numbers, arrays, and objects without interpreting templates or
directives inside the inserted data. It does not apply an encoding view.

`{"$literal": ...}` copies its entire payload without rendering. For example,
`{"$literal":{"$value":"params.name"}}` produces the literal object
`{"$value":"params.name"}`. An object containing `$value` or `$literal` alongside
other keys is an ordinary object. Structured output uses deterministic key order
and owns its bytes independently of the source and context.

## Limits and verification

Even a short template can produce a large body when it repeats input values.
Nested sections can also multiply the work without producing much output.
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
