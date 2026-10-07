// Package tools owns reusable authored Tool contracts. Execution, resource
// management, and endpoint-specific transports are implemented separately.
package tools

// SchemaIDTools identifies the authored ToolDefinition JSON schema. The schema
// is embedded automatically by internal/schema for offline compilation.
const SchemaIDTools = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/tools/schema.json"
