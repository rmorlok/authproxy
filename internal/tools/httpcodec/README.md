# Tool HTTP request encoding

An HTTP Tool sends caller arguments in query parameters or a request body. This
package compiles their templates once, then renders and encodes them for each
invocation. It supports query parameters, JSON, text, base64 binary, form fields,
and multipart uploads. It returns bytes and content types without making network
requests.

Query parameters and form fields use `application/x-www-form-urlencoded`
encoding. The same encoding works in a URL's query string and a form body.

For example, a CRM search Tool can declare these `proxyHttp.query` fields:

```json
{
  "tenant": "{{cfg.tenant}}",
  "search": "{{params.search}}",
  "tag": {"$value": "params.tags"},
  "active": {"$value": "params.active"}
}
```

Given tenant `acme`, search text `Ada & Bob`, tags `["lead", "web"]`, and
`active: false`, the encoded result is:

```text
active=false&search=Ada+%26+Bob&tag=lead&tag=web&tenant=acme
```

The ampersand belongs to the search text, so it becomes `%26`. The tags produce
two `tag` parameters in their original order. The same field definitions can
be supplied as `proxyHttp.form.fields` to produce the same text as a form body.

## Compile once, render per invocation

Use `CompileParameters` for authored field templates. It takes the existing
`map[string]common.RawJSON` query/form contract and `render.Limits`. Keep the
program for the definition revision, then render it against each invocation's
captured context:

```go
limits := render.Limits{MaxBytes: 64 * 1024, MaxDepth: 32, MaxSteps: 10000}
program, err := httpcodec.CompileParameters(map[string]common.RawJSON{
    "tenant": common.RawJSON(`"{{cfg.tenant}}"`),
    "search": common.RawJSON(`"{{params.search}}"`),
    "tag":    common.RawJSON(`{"$value":"params.tags"}`),
    "active": common.RawJSON(`{"$value":"params.active"}`),
}, limits)
if err != nil {
    return err
}

data, err := render.NewContext(render.Input{
    Cfg: map[string]any{"tenant": "acme"},
    Params: map[string]any{
        "search": "Ada & Bob",
        "tags":   []any{"lead", "web"},
        "active": false,
    },
}, limits)
if err != nil {
    return err
}
encoded, err := program.Render(ctx, data)
if err != nil {
    return err
}
// encoded contains the query/form text shown above.
```

Here `ctx` is the invocation's `context.Context`; the packages are
`internal/schema/common`, `internal/tools/render`, and `internal/tools/httpcodec`
from this module. The limits are examples; callers supply their effective
execution policy.

Compiled programs own their source and can be reused concurrently. Field names
must be nonempty and stay literal, even if they contain Mustache syntax or are
named `$value`.
All field values are compiled as one structured program, so they share rendering
work and output limits. See [the renderer README](../render/README.md) for string
templates and typed directives. Compilation checks template syntax; rendering
also checks that the resulting values have a valid parameter shape.

## Optional fields

Use `$optional` for a whole field that should be absent when its input is
missing or null. For example, a paginated request can use:

```json
{
  "cursor": {"$optional": "params.cursor"},
  "limit": 10
}
```

The first call can supply `{}` or `{"cursor": null}`; both produce `limit=10`.
A later call with `{"cursor": "page+2"}` produces
`cursor=page%2B2&limit=10`. False, zero, and an empty string are present values:
an empty cursor produces `cursor=&limit=10`.

`$optional` must be the field's entire value and its only key. It takes a plain
dotted reference, like `$value`. It cannot appear inside a repeated-value array
or a JSON body template. A present value uses the ordinary scalar/repeated-value
rules below; nulls inside a present array still encode as `null`.
Ordinary `$value` remains strict: a missing reference is an error, and a present
null encodes as the text `null`. Marking one lookup optional does not change
other lookups of the same input.

## Encoding rules

| Resolved JSON value | Encoding for a field named `value` |
| --- | --- |
| `"Ada & Bob"` | `value=Ada+%26+Bob` |
| `""` | `value=` |
| `false` | `value=false` |
| `9007199254740993` | `value=9007199254740993` |
| `null` | `value=null` |
| `["first", "second"]` | `value=first&value=second` |
| `[]` | No parameter emitted. |
| Object or nested array | Error. |

Keys sort lexically, and repeated values retain their order. Numbers keep their
JSON spelling without conversion to floating point. Null uses the text `null`,
matching ordinary scalar Tool interpolation; it does not omit the field.
Empty strings and null therefore remain different values on the wire.

Supply unencoded names and values. In particular, use `{{params.search}}` in
structured fields, not `{{query.params.search}}`. A literal `%2F` is encoded as
`%252F` so the provider receives the literal text `%2F`. Values inserted with
`$value` remain data and are never interpreted as templates a second time.

Call `EncodeParameters(ctx, fields, maxBytes)` when a request planner already
has resolved `map[string]common.RawJSON` values. This entry point applies the
same scalar, repeated-value, and escaping rules without template evaluation.
It rejects directive objects just as it rejects other objects.
Each map entry must contain a complete JSON value: use `common.RawJSON("null")`
for null. Nil or empty byte slices are invalid; omit the key to omit a field.

## Request bodies

`CompileBody` takes the body fields of a `tools.ProxyHTTP` definition. Choose at
most one of `bodyJson`, `bodyTemplate`, `bodyRaw`, `form`, or `multipart`.

### JSON

For a CRM contact update, use typed insertion to preserve booleans and nulls:

```yaml
bodyJson:
  name: "{{params.name}}"
  active:
    $value: params.active
  manager:
    $value: params.manager
```

Arguments `{"name":"Ada","active":false,"manager":null}` produce
`{"active":false,"manager":null,"name":"Ada"}` with content type
`application/json`. String values are JSON-escaped automatically. Missing
required references fail before a body is returned.

### Text templates

Use `bodyTemplate` when the entire payload is authored as text:

```yaml
bodyTemplate:
  mediaType: text/plain; charset=utf-8
  template: "Contact {{params.name}} is ready."
```

For a JSON text template, use the renderer's `json` view to insert a complete,
escaped JSON value, including quotes around strings:

```yaml
bodyTemplate:
  mediaType: application/json
  template: '{"name":{{json.params.name}}}'
```

The codec checks the final text is valid JSON for media subtypes `json` and
`+json`, including types such as `application/problem+json`. Other media types
retain the rendered text as-is. Media types and their parameters are literal;
they are never evaluated as templates.

### Binary and form bodies

For binary data, supply standard base64 in `bodyRaw`:

```yaml
bodyRaw: "{{params.fileBase64}}"
```

An argument of `AP+A` produces the three bytes `00 ff 80`. This mode leaves the
content type to the request headers. It accepts standard padded base64, including
CR/LF line breaks, and rejects invalid padding bits, URL-safe variants, and
missing required padding. The base64 text is never a filename or a template
source to evaluate again.

For a form body, put the query-style field definitions under `form.fields`:

```yaml
form:
  fields:
    search: "{{params.search}}"
    tag:
      $value: params.tags
    cursor:
      $optional: params.cursor
```

This uses the parameter rules above and returns content type
`application/x-www-form-urlencoded`.

### Multipart uploads

A document upload can combine a text comment with binary file content:

```yaml
multipart:
  parts:
    - name: comment
      text: "{{params.comment}}"
    - name: upload
      filename: "{{params.filename}}"
      mediaType: application/pdf
      bodyRaw: "{{params.fileBase64}}"
```

Each part has exactly one of `text` or `bodyRaw`. Names, filenames, and payloads
can use templates; media types stay literal. A filename is metadata sent to the
provider, never a path to read from disk. Part order and repeated names are
preserved. Text defaults to `text/plain; charset=utf-8`; binary defaults to
`application/octet-stream`. Rendered names and filenames must be valid UTF-8
without control characters, and names must be nonempty.

A compiled body does not support `mediaType: "{{params.contentType}}"`. The upload
above sends `application/pdf` on every invocation, even when the supplied bytes
contain a PNG. Use separate definitions for specific types, or omit `mediaType`
to send `application/octet-stream` when the provider accepts generic binary data.
The codec does not infer a media type from the filename or file contents.

The returned `multipart/form-data` content type includes a generated boundary.
Request assembly must send that exact content type with the returned bytes.

### Compiling and rendering a body

Using the limits and invocation context from the earlier example:

```go
bodyProgram, err := httpcodec.CompileBody(&tools.ProxyHTTP{
    BodyJSON: common.RawJSON(`{
        "search":"{{params.search}}",
        "active":{"$value":"params.active"}
    }`),
}, limits)
if err != nil {
    return err
}
body, err := bodyProgram.Render(ctx, data)
if err != nil {
    return err
}
// body.Present is true, body.ContentType is "application/json", and
// body.Bytes contains {"active":false,"search":"Ada & Bob"}.
```

Here `tools` is `internal/schema/resources/tools`. Compilation checks only body
fields; method, URL, headers, query, and response rules are separate concerns.
Compiled bodies own their source and support concurrent rendering. Each result
owns its bytes.

`Present` distinguishes an omitted body from a present empty text, binary, or
form body. `bodyJson: null` is present and produces the four bytes `null`.
Explicit nulls for the other body modes are invalid.

`EncodeMultipart` and `DecodeBase64` are also available for already-resolved
input. They apply the same encoding rules without interpreting templates.

## Bounds and integration

`CompileParameters` requires positive `render.Limits`. `MaxBytes` independently
bounds the combined source, intermediate rendered JSON, and encoded output.
Source accounting includes literal field-name bytes and the JSON values framed
as an array. `MaxDepth` and `MaxSteps` apply across all compiled field values.
`EncodeParameters` takes a positive byte cap for the same source accounting and
encoded output. Percent escaping and repeated names count toward the output cap.
These checks use the **same numeric cap**, not separate source and encoded-size
settings. A value can fit the JSON limit and still exceed the encoded limit:
each UTF-8 byte in a Chinese character becomes three bytes (`%XX`), so about
25 KiB of such text becomes about 75 KiB in the query and exceeds a 64 KiB cap.
Repeated parameters also repeat their field names. Choose `MaxBytes` (or
`maxBytes`) for the largest encoded output the execution policy permits; fitting
the source limit alone does not guarantee that encoding will succeed.
Temporary parsing and encoding allocations are proportional to these limits;
the limits are not a hard memory quota.

`CompileBody` also requires positive limits. `MaxBytes` separately caps the
source strings, serialized template source, rendered intermediate values,
and final body. All multipart fields share one rendering budget. Their
intermediate JSON representation counts escaping and field names; their final
MIME body counts part headers, boundaries, and the closing delimiter. A small
payload can therefore exceed the cap after framing. `MaxDepth` and `MaxSteps`
apply to the compiled render program, not independently to each part.

`EncodeMultipart` takes one positive cap for source and final MIME bytes.
`DecodeBase64` takes separate positive caps for encoded and decoded bytes; body
compilation uses `MaxBytes` for both. Encoded CR/LF bytes count toward its input
cap even though the decoder ignores them.

Cancellation and errors return no partial encoded result. Encoding errors do
not include resolved configuration values. The codecs do no network or file I/O.
Request assembly must validate the final URL and headers, decide how structured
fields combine with an existing query, and reconcile body content types with
authored headers while preserving multipart boundaries. Authorization,
input-schema validation, credentials, request
sending, and response handling belong to the execution layer. This package
does not yet assemble or execute complete Tool requests.

Run `go test -race ./internal/tools/httpcodec ./internal/tools/render` for the
codec and renderer tests.
