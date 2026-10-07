// Package toolsets owns generation-addressed ToolSet resource contracts and
// explicit tool templates. Management, reconciliation, and execution are
// implemented separately from these serialized definitions.
package toolsets

// SchemaIDToolSets identifies the canonical ToolSet resource JSON schema. The
// schema is embedded by internal/schema for compilation without external I/O.
const SchemaIDToolSets = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/toolsets/schema.json"
