package tools

import "github.com/rmorlok/authproxy/internal/schema/common"

// ToolDefinition is the reusable behavior of an authored tool, independent of
// its resource identity or connection binding. Standalone Tools and explicit
// ToolSet templates share this contract. Exactly one executor is required;
// imported OpenAPI and MCP execution plans are not authorable definitions.
type ToolDefinition struct {
	Description string         `json:"description" yaml:"description"`
	Verbs       []string       `json:"verbs" yaml:"verbs"`
	InputSchema common.RawJSON `json:"inputSchema" yaml:"inputSchema"`
	// OutputSchema validates successful structured results. Omission disables
	// that check; a boolean JSON Schema remains distinct from omission.
	OutputSchema common.RawJSON   `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty"`
	Hints        *BehavioralHints `json:"hints,omitempty" yaml:"hints,omitempty"`
	Limits       *ExecutionLimits `json:"limits,omitempty" yaml:"limits,omitempty"`
	ProxyHTTP    *ProxyHTTP       `json:"proxyHttp,omitempty" yaml:"proxyHttp,omitempty"`
	// Javascript declares async function execute(params, context). Contract
	// validation checks presence; the execution compiler validates the source.
	Javascript *string `json:"javascript,omitempty" yaml:"javascript,omitempty"`
}

// BehavioralHints are descriptive claims made by an administrator. They never
// grant permission or enable retries. Pointers distinguish an omitted hint
// from an explicit false value; provider provenance belongs to import results.
type BehavioralHints struct {
	ReadOnly    *bool `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	Destructive *bool `json:"destructive,omitempty" yaml:"destructive,omitempty"`
	Idempotent  *bool `json:"idempotent,omitempty" yaml:"idempotent,omitempty"`
}

// ExecutionLimits are optional positive per-tool caps. Omitted values inherit
// execution policy; this contract supplies no defaults and cannot raise an
// operator ceiling. Milliseconds and byte counts are exact integer units.
type ExecutionLimits struct {
	TimeoutMillis *int64 `json:"timeoutMillis,omitempty" yaml:"timeoutMillis,omitempty"`
	MaxRequests   *int   `json:"maxRequests,omitempty" yaml:"maxRequests,omitempty"`
	MaxInputBytes *int64 `json:"maxInputBytes,omitempty" yaml:"maxInputBytes,omitempty"`
	// MaxOutputBytes caps decoded result content; MaxResponseBytes caps the
	// aggregate decoded upstream responses. The runtime may also impose a
	// shared aggregate budget across both categories.
	MaxOutputBytes   *int64 `json:"maxOutputBytes,omitempty" yaml:"maxOutputBytes,omitempty"`
	MaxResponseBytes *int64 `json:"maxResponseBytes,omitempty" yaml:"maxResponseBytes,omitempty"`
	// MaxEncodedResultBytes separately bounds the serialized result envelope,
	// including expansion caused by base64-encoded rich content.
	MaxEncodedResultBytes *int64 `json:"maxEncodedResultBytes,omitempty" yaml:"maxEncodedResultBytes,omitempty"`
	MaxStackDepth         *int   `json:"maxStackDepth,omitempty" yaml:"maxStackDepth,omitempty"`
}
