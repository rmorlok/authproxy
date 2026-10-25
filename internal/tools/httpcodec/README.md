# Tool HTTP requests and results

An HTTP Tool sends caller arguments in query parameters or a request body. This
package compiles request templates and response rules once, then applies them
for each invocation. It supports query parameters, JSON, text, base64 binary,
form fields, and multipart uploads. It can prepare a complete outbound
`http.Request` and map a buffered upstream response into a rich Tool result,
without making network requests.

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

## Complete requests

`CompileRequest` combines the envelope, query, and body codecs. For example, a
CRM contact update can declare:

```yaml
method: PATCH
url: "https://{{cfg.apiHost}}/v1/contacts/{{path.params.contactId}}?audit=1"
headers:
  X-Tenant: "{{cfg.tenant}}"
query:
  tag:
    $value: params.tags
bodyJson:
  name: "{{params.name}}"
```

With `cfg.apiHost` set to `crm.example.com`, tenant `acme`, contact ID `sales/42`,
tags `["lead", "web"]`, and name `Ada`, the prepared request contains:

```http
PATCH /v1/contacts/sales%2F42?audit=1&tag=lead&tag=web
Host: crm.example.com
X-Tenant: acme
Content-Type: application/json

{"name":"Ada"}
```

After decoding the definition into a `*tools.ProxyHTTP` and capturing invocation
data with `render.NewContext`, compile once and render each invocation:

```go
requestProgram, err := httpcodec.CompileRequest(definition, limits)
if err != nil {
    return err
}
outbound, err := requestProgram.Render(ctx, data)
if err != nil {
    return err
}
// outbound is ready for the connection's streaming proxy. No request has
// been sent and no credentials have been applied.
```

Each result owns its URL, headers, and body reader. The caller must close the
body or transfer it to the proxy. Present bodies have `GetBody` readers for the
proxy's existing authentication-recovery behavior; each reader starts at byte
zero. Absent bodies have a nil reader, while present empty bodies use
`http.NoBody`. The method is a literal HTTP token, including extension methods;
URL and header values can use templates. Header names stay literal.

Final validation and combination rules are:

| Component | Rule |
| --- | --- |
| URL | Absolute HTTP or HTTPS with a valid authority. Reject fragments, opaque URLs, unescaped whitespace/control bytes, and malformed escapes. Preserve existing path/query spelling, including semicolons in query data. |
| Structured query | Append encoded fields after the existing query. Reject a key present in both sources; repeated values within one source are allowed. An omitted `$optional` field does not collide. |
| Header names | HTTP tokens, unique without regard to case. Reject hop-by-hop and framing headers such as `Connection`, `Transfer-Encoding`, `Trailer`, and `Content-Length`; the transport handles framing. |
| Header values | Reject control bytes other than horizontal tabs. Validate a rendered `Content-Type` as a concrete media type. |
| Host override | A `Host` header template becomes `outbound.Host`, separate from ordinary headers. It changes the HTTP authority; the URL still selects the network destination and TLS server name. |
| Credentials | Preserve authored authorization headers and query values. Connection credentials are applied later by the proxy with its existing precedence. |

For example, `url: "https://{{cfg.apiHost}}/upload"` with
`headers: {Host: "{{cfg.virtualHost}}"}` can connect to one configured host while
sending another HTTP Host value. The codec performs no DNS lookup or destination
allowlisting; deployment egress policy still applies at execution.

### Choosing URL interpolation

Raw interpolation inserts URL syntax. Use it only when the trusted Tool author
intends the value to control that part of the URL. For example,
`https://{{cfg.apiHost}}/v1/items` selects a host from connection configuration;
`https://{{params.host}}/v1/items` deliberately lets an argument select it.
The compiler validates the final URL's syntax; it does not pin a destination or
infer which URL components an argument was intended to control. The proxy later
applies the connection's credentials to that destination. Configuration can also
come from setup input, so selecting a value from `cfg` does not validate it.

Keep the `/` between an authority and an interpolated path. In
`https://api.example.com{{params.suffix}}`, the suffix is still part of the
authority: `@other.example/x` changes the destination to `other.example` and
turns `api.example.com` into URL userinfo. A fixed prefix alone does not fix the
host. Neither the `path` nor `query` encoding view validates hostnames; constrain
host inputs when an author deliberately allows them to vary.

For an argument that represents one item ID, write:

```yaml
url: "https://api.example.com/v1/items/{{path.params.id}}"
query:
  search:
    $value: params.search
```

An ID of `42?include=secrets&admin=true` becomes
`42%3Finclude=secrets&admin=true` in the path, without creating query parameters.
An ID of `../../admin` becomes `..%2F..%2Fadmin`. Structured query fields encode
their values automatically; use `{{query.params.search}}` if writing a query
value directly in the URL instead. Raw `{{params.id}}` would insert `?`, `&`,
and `/` as URL syntax. The `path` view rejects empty values and the dot segments
`.` and `..`. Encoding still does not decide which resource an argument may
identify, and providers can apply their own path normalization. Use the Tool's
input schema to constrain identifiers to the intended set.

### Body content type

Body codecs supply a default `Content-Type`. An authored header may add
parameters but cannot contradict declared body parameters. Structured JSON may
use a JSON media type such as `application/vnd.example+json`; text templates and
forms must retain their declared base media type. Multipart accepts an authored
`multipart/form-data` header, then adds the generated boundary. An authored
boundary is rejected because it cannot describe the generated body. Raw or absent
bodies use the authored media type, if supplied. Literal Content-Type conflicts
fail at compilation; templated values receive the same check after rendering.

`CompileRequest` covers outbound request fields. Compile response rules separately
with `CompileResponse`, described below. Authorization, input-schema validation,
credential capture, sending, and response-stream handling belong to the executor.

## Response results

`CompileResponse` captures HTTP status rules and the limits for mapping one
buffered response. For example, a CRM Tool can describe two safe errors:

```yaml
response:
  errors:
    - statuses: [404, 410]
      code: CONTACT_MISSING
      message: The contact is no longer available.
    - statusClass: 4
      code: PROVIDER_REQUEST_REJECTED
      message: The provider rejected the request.
```

A 404 or 410 becomes `CONTACT_MISSING`; another 4xx becomes
`PROVIDER_REQUEST_REJECTED`. A 200 response containing `{"id":"contact-42"}`
becomes `structuredContent` with that JSON value. A 503 becomes the generic
`UPSTREAM_FAILED` error. Provider error bodies, status text, and arbitrary
headers are never copied into these public errors. Status failures include only
the HTTP status in `error.details`, such as `{"status":404}`.

Exact status rules take precedence over class rules, then a `default: true`
rule. Rule order does not change the result. A default rule matches only non-2xx
statuses, so adding a catch-all error preserves successful responses. Use an
exact status or `statusClass: 2` to deliberately map a 2xx status to an error.
Without a matching rule, only 2xx is successful. Overlapping selectors at the
same priority and reserved platform error codes fail compilation. An authored
`retryable: true` is advice; this package never retries a request.

After the executor has read and decoded the upstream body, it can map the result:

```go
program, err := httpcodec.CompileResponse(definition.Response, httpcodec.ResponseLimits{
    MaxSourceBytes:        4096,
    MaxResponseBytes:      1024 * 1024,
    MaxOutputBytes:        512 * 1024,
    MaxEncodedResultBytes: 1024 * 1024,
})
if err != nil {
    return err
}
result, err := program.Map(ctx, httpcodec.ResponseInput{
    StatusCode:  upstreamStatus,
    ContentType: upstreamContentType,
    Body:        decodedBody,
    ToolID:      "tol_01example0000001",
    Revision:    7,
    InvocationID: "tiv_01example0000001",
    Metadata: api.ToolResultMetadata{
        ConnectionID: "cxn_01example0000001",
    },
    ExecutionState: api.ToolExecutionStateMayHaveExecuted,
})
```

Here `api` is `internal/schema/api`. Identities and source revisions come from
the captured invocation. The executor supplies its known execution state:
`mayHaveExecuted` or `completed`. Every mapped error preserves that state;
HTTP status alone cannot establish whether upstream side effects occurred.
`notStarted` is invalid when mapping an already-received response.

Successful responses without a transform use these representations:

| Body and Content-Type | Tool result |
| --- | --- |
| Empty bytes, including with a JSON content type | Empty `content` array; no `structuredContent`. |
| `application/json` or another `json` / `+json` subtype | `structuredContent`, preserving scalars, large numbers, and explicit JSON `null`. |
| `text/*` with UTF-8, US-ASCII, or no declared charset | A text block. Invalid bytes fail instead of being replaced. |
| `image/*` or `audio/*` | A typed block with base64 data and the declared media type. |
| Other media, or text with an unsupported charset | An embedded blob resource preserving bytes and the declared media type. |
| No Content-Type | An embedded `application/octet-stream` blob; no content sniffing. |

The JSON subtype rule takes precedence, including for `text/json`. JSON bytes
must be valid UTF-8 and contain exactly one JSON value. If a Content-Type has a
valid base type but a recoverable parameter syntax error, such as
`application/json; charset`, the mapper drops the parameters and uses the base
type. Text with uncertain charset parameters becomes a blob to preserve its
bytes. An invalid base type, conflicting duplicate parameters, or invalid header
bytes still produce `UPSTREAM_PROTOCOL_ERROR`; malformed JSON or supported text
becomes `OUTPUT_INVALID`. Content decoding happens only after status
classification, so a status error does not depend on the provider's error-body
format.

This mapper returns AuthProxy's transport-neutral `ToolResult`. JSON arrays,
scalars, and null retain their original shape, without a duplicate text block.
An adapter must render structured data for clients that need model-facing text;
the Python integration must also retain the complete result as an artifact.
A future MCP server adapter must apply the negotiated protocol's result
requirements at that boundary, including text compatibility and any required
object wrapping.

Generated blob URIs have the form
`urn:authproxy:tool-invocation:tiv_01example0000001:response`. They identify inline
content; they are not download endpoints. Input bytes and metadata pointers are
copied into independent results. Every result has a nonnil `content` array,
including JSON-only and error results.

The executor owns response reading, decompression, cancellation of network I/O,
aggregate invocation accounting, and output-schema validation. `Map` operates
only on supplied bytes and does not access the network or close a stream.

### Response transforms

A pure JavaScript expression can select or reshape a successful response. For
example, a CRM returns `{"id":"contact-42","profile":{"name":"Ada"},"debug":{"trace":"provider-only"}}`,
but the Tool needs only the contact's identity and its tenant:

```yaml
response:
  transformJavascript: |
    ({ id: response.body.id, name: response.body.profile.name, tenant: cfg.tenant })
```

With `cfg.tenant` captured as `"acme"`, the result's `structuredContent` is
`{"id":"contact-42","name":"Ada","tenant":"acme"}`. The full upstream body must fit
the response input cap; only the smaller projection must fit the output cap.
Compilation checks expression syntax without running the expression. Each
mapping uses a fresh runtime with copied `params`, `context`, `cfg`, `labels`,
and `annotations`, plus this `response` object:

| Field | Value available to the expression |
| --- | --- |
| `status` | The received HTTP status number. |
| `headers` | Lowercase names mapped to arrays of strings, such as `{"x-request-id":["req-7"]}`. Repeated values retain their order; empty lists stay arrays. |
| `mediaType` | The effective Content-Type, including valid parameters, with the same fallback and repair rules as passthrough. |
| `body` | Decoded JSON, a supported text string, or `{encoding:"base64",data:"..."}` for binary content, including images, audio, and unsupported text charsets. Empty content omits this field; JSON `null` remains explicit null. |

For example, `response.headers["x-request-id"]?.[0] ?? null` selects an optional
request ID. A header whose name is not a valid HTTP token, or with any value
that is not valid UTF-8 or contains prohibited HTTP control characters, is
omitted from `response.headers` instead of failing the response. Omitted
headers still count toward the input allowance. Duplicate names differing
only in case are rejected. `ResponseInput.ContentType` controls body decoding;
the supplied header map is metadata and does not override it.

Set positive runtime limits before compiling a transform:

```go
limits.Transform = javascript.Limits{
    MaxSourceBytes: 4096,
    MaxInputBytes: 2 * 1024 * 1024,
    MaxOutputBytes: 1024 * 1024,
    MaxDepth: 32,
    MaxNodes: 10000,
    MaxStackDepth: 128,
    Timeout: time.Second,
}
program, err := httpcodec.CompileResponse(definition.Response, limits)
```

Here `javascript` is `internal/tools/javascript`. On each `Map` call, supply
`ResponseInput.Context` from `render.NewContext` and `ResponseInput.Headers`
from the captured upstream response. The executor must use the same admission
snapshot for the context and result identities. Neither field is needed for
passthrough. Status errors run first and bypass the transform, including its
context, header, and body checks; an error body never becomes a success projection.

JSON-compatible results become `structuredContent`. A string also produces a
text block with the same value; other values are not duplicated as text. Empty
input and JSON null remain distinguishable with `"body" in response`. Returning
an absent body directly returns `undefined` and fails; an expression can instead
return an explicit fallback. A top-level string containing an unpaired UTF-16
surrogate fails because a UTF-8 text block cannot preserve it. Nested strings
retain the expression runtime's JSON representation.

JavaScript uses Number precision: supply exact identifiers as strings before
parsing. A projection cannot restore digits already rounded by JavaScript.
Without a transform, JSON passthrough continues to preserve original numeric
tokens. Transforms expose no network access or rich-result host helpers.

Invalid JSON/text, unsupported JavaScript values, and thrown exceptions become
safe `OUTPUT_INVALID` errors; malformed HTTP metadata produces
`UPSTREAM_PROTOCOL_ERROR`. Runtime input, output, traversal, stack, or timeout
limits become `EXECUTION_LIMIT_EXCEEDED`. The separate response output and
envelope caps produce `OUTPUT_TOO_LARGE`. Caller cancellation retains its Go
context error. All mapped failures preserve the supplied execution state and
contain no partial output, source text, thrown values, or private input data.

### Response limits

The four response limits must be positive. `Transform` limits are additionally
required when an expression is configured:

- `MaxSourceBytes` bounds the combined expression source, rule codes, messages,
  and selector data before compilation. It does not count JSON/YAML framing.
- `MaxResponseBytes` bounds the supplied decoded body on the success path.
  Content-Type text is independently capped at the same size before parsing.
  Status errors use neither field, so unused content cannot replace a status
  error or its retry advice. The executor must still enforce read/decompression
  bounds before allocating buffers and account for all responses across a
  multi-request invocation.
- `MaxOutputBytes` bounds successful raw JSON, UTF-8 text, or decoded binary
  content before wrapping it in an envelope or encoding it as base64. With a
  transform, it bounds the returned JSON instead of the upstream body.
- `MaxEncodedResultBytes` bounds the complete serialized `ToolResult`, including
  identities, metadata, JSON escaping, and base64 expansion. It does not include
  any enclosing API action response.
- `Transform.MaxInputBytes` bounds serialized response metadata/body preparation
  and, in the runtime, its combined size with the captured context. Header
  counts and raw values are checked before normalization and escaping; serialized
  framing and base64 expansion count toward this allowance. Runtime structural
  limits apply across the response and context. Runtime output bytes count the
  expression's JSON; the response caps separately bound content and the final
  envelope, including both copies of a string result.

Size failures become `OUTPUT_TOO_LARGE` without truncated content. An oversized
result, including an authored error message, is replaced once by a safe error.
If even that envelope cannot fit, `Map` returns `ErrEncodedResultLimit` with no
result. Invalid invocation metadata and cancellation also return Go errors with
no partial result. These caps bound processing; they are not hard memory quotas.

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
`CompileRequest` uses three bounded programs: one for the URL and all header
values together, one for query values, and one for the body. `MaxDepth` and
`MaxSteps` apply per program, so the entire request can use up to three
`MaxSteps` allowances. Adding headers does not create a separate allowance per
header.

The complete-request compiler also caps combined raw source bytes and final
prepared data with `MaxBytes`. Source counting includes the method, URL, header
names/values, query names/JSON values, and all body source fields. Final counting
includes the method, serialized URL, explicitly authored Host override, header
names/values (including generated content types), and body bytes. The default
Host is already represented by the URL and is not counted again. An explicitly
authored Host counts even when it equals the URL authority. This is a prepared-data cap,
not an exact wire-size cap: credentials, transport framing, and automatically
added headers come later. A request can fit every component limit but exceed the
combined limit; failures return no partial request.

Run `go test -race ./internal/tools/httpcodec ./internal/tools/render` for the
codec and renderer tests.
