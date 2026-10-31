# Common Schema Types

This package contains reusable primitives used by other schema packages, including resource names, string and integer value sources, images, human-readable durations and byte sizes, request types, target-send evidence, raw JSON helpers, and setup-form JSON Schema / UI Schema helper subpackages.

Types here should be broadly reusable across AuthProxy resources, APIs, and
configuration sections rather than owned by one specific contract.

`SendEvidence` is the shared `notSent` / `mayHaveSent` / `sent` vocabulary for
proxy HTTP-client observations and public response metadata. The HTTP client
package re-exports it as `SendState` for compatibility; endpoint-specific
envelopes reference this primitive directly. A zero value is not evidence that
the target was unsent.
