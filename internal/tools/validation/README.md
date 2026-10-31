# Tool argument and output validation

This package checks caller arguments and successful structured output against a
Tool's JSON schemas. Compile a `Program` for a definition revision and reuse it
for that revision's invocations. It uses the same JSON Schema 2020-12 compiler
as Tool authoring, including local references and the ban on loading schemas
from the network or filesystem.

For example, a contact lookup can require an integer ID and return a name:

```go
program, err := validation.Compile(
    common.RawJSON(`{
        "type": "object",
        "required": ["id"],
        "properties": {"id": {"type": "integer"}},
        "additionalProperties": false
    }`),
    common.RawJSON(`{
        "type": "object",
        "required": ["name"],
        "properties": {"name": {"type": "string"}}
    }`),
    validation.Limits{
        MaxInputBytes: 64 * 1024,
        MaxOutputBytes: 1024 * 1024,
        MaxDepth: 32,
        MaxValues: 10000,
        MaxNumberDigits: 1024,
    },
)
if err != nil {
    return err
}

params, err := program.ValidateInput(ctx, common.RawJSON(`{"id":9007199254740993}`))
if err != nil {
    return err
}
// params["id"] is json.Number("9007199254740993"). Pass params to
// render.NewContext before preparing and sending the connection request.

// After execution, check only successful structuredContent.
err = program.ValidateOutput(ctx, common.RawJSON(`{"name":"Ada"}`))
```

The imports are `internal/tools/validation`, `internal/schema/common`, and
`internal/tools/render` from this module. The limits above are examples; the
invocation layer supplies its effective execution policy.

## Values and ownership

Arguments must contain exactly one JSON object. `{"id":"42"}` fails the
example schema because a string is not an integer. Validation never converts
types, inserts schema defaults, or removes properties. Allowed extra properties
remain in the returned map. JSON numbers keep their exact spelling and precision.

Each successful input check returns a new map and nested values. The compiled
program does not retain it or the supplied JSON bytes. Callers may change their
schema bytes after compilation and may reuse a program concurrently. Do not
modify an argument or output buffer while a validation call is reading it.
Malformed JSON, invalid UTF-8 bytes, trailing values, and duplicate object keys
are rejected. Escaped Unicode strings use Go's `encoding/json` decoding rules.

Output checking applies to `ToolResult.StructuredContent`, not text/image/audio
blocks or the whole result envelope. Call it only for successful results. A
missing output schema disables this check. A configured schema requires present
JSON: `nil` is missing, while `null` is a value. Thus schema `true` accepts
explicit `null` but rejects missing content; schema `false` rejects every value.
Validation leaves the original output bytes unchanged.

## Limits and errors

All limits must be positive. Input and output have separate encoded byte caps,
including whitespace. Depth counts nested objects and arrays, starting at one
for a root container. The value allowance counts containers and scalar values;
object keys consume bytes but are not separate values.

The number allowance counts mantissa digits plus the absolute decimal exponent.
For example, `12e-3` consumes five digits of allowance. `MaxNumberDigits` must not
exceed 4096, the maximum rational expansion allowance. This keeps numbers within
the range supported by the schema library's rational-number conversions and
bounds the expansion used by exact numeric comparisons: a short input such as
`1e999999999` cannot trigger an enormous rational-number allocation. Accepted
numbers are not rounded or rewritten.

Runtime errors are fixed categories: `ErrInvalidInput`, `ErrInvalidOutput`,
`ErrLimit`, or a context cancellation/deadline error. They do not wrap parser or
schema diagnostics that could expose private values. Compilation errors describe
trusted authored schemas and are intended for definition handling.

Cancellation is checked while decoding and before and after schema evaluation.
The schema library evaluates synchronously and has no interruption or evaluation
step budget. These limits bound decoded data, **not** reference/composition
expansion or evaluation time. Administrators remain responsible for the schemas
they author. A true evaluation-work bound is still needed before this is used by
public Tool invocation; returning early from a detached validation goroutine
would leave that work running.

The caller owns authorization, revision selection/caching, request execution,
and result formation. Invalid input must stop execution. Invalid output may
follow an upstream side effect, so the eventual executor must preserve execution
state when forming `OUTPUT_INVALID` and must not retry automatically. The package
does not publish routes or change existing Tool management behavior.

Run `go test -race ./internal/tools/validation ./internal/schema/resources/tools`
for validation and native compiler coverage.
