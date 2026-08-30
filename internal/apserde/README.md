# API Serialization

`internal/apserde` owns AuthProxy's safe JSON and YAML serialization rules for
API values. Its primary responsibility is preventing fields tagged with
`apiredact:"secret"` from leaking through HTTP responses while preserving the
normal wire shape of the surrounding value.

## Broad approach

The package first checks whether a value tree contains any redaction tags. If
it does not, or the request context has explicitly authorized secret replay,
the ordinary JSON or YAML encoder supplies the wire representation. This fast
path preserves custom marshalers without paying the reflection cost of walking
every field.

When redaction is required, the serializer recursively converts the value into
a plain map/slice/scalar tree. It follows JSON or YAML field names,
`omitempty`, anonymous or explicitly inline structs, maps, slices, pointers,
and AuthProxy's polymorphic `InnerVal` wrappers. Values on secret-tagged fields
are replaced with the same number of `*` runes. `Report.Redacted` tells the HTTP
renderer to set `X-AuthProxy-Data-Redacted: true`.

The reverse guard, `ValidateNoRedactedPlaceholders`, rejects mask-only values
on secret-tagged request fields. This prevents a client from reading a
redacted response and accidentally writing the placeholder back as a real
credential. It reports JSON-style paths for every offending value, including
values nested in inline structs, slices, arrays, and maps.

Secret replay is an authorization result, not an authorization decision made
by this package. A caller must first authorize the `secrets:replay` permission
and only then use `WithSecretReplay` on the request context.

## Usage

```go
type response struct {
    Name   string `json:"name" yaml:"name"`
    Secret string `json:"secret" yaml:"secret" apiredact:"secret"`
}

data, report, err := apserde.MarshalJSONForAPI(ctx, response{
    Name:   "example",
    Secret: "token",
})
// data contains {"name":"example","secret":"*****"}.
// report.Redacted is true.
```

API handlers normally use `apgin.APIJSON`, which calls this package and sets
the redaction header. Resource/action request binders in `internal/apgin` call
`ValidateNoRedactedPlaceholders` before lifecycle validation.

## Adding secret fields

- Put `apiredact:"secret"` on the schema field that owns the sensitive value.
- Keep its JSON and YAML tags aligned with the public contract.
- Add round-trip tests for the concrete value shape, especially custom
  marshalers and polymorphic wrappers.
- Never call `WithSecretReplay` unless the request has already passed the
  corresponding permission check.
