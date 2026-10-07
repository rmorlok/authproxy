// Package tools owns authored Tool definitions and resource contracts.
// Execution, resource management, and endpoint-specific transports are
// implemented separately.
package tools

// SchemaIDTools identifies the authored ToolDefinition JSON schema. The schema
// is embedded automatically by internal/schema for offline compilation.
const SchemaIDTools = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/tools/schema.json"

// SchemaIDToolResource identifies the Tool resource envelope, including
// connection identity and server-owned revision/management status.
const SchemaIDToolResource = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/tools/schema-resource.json"

// SchemaIDToolPatch identifies a partial Tool update; complete executable-plan
// validation occurs after applying the patch to the current resource.
const SchemaIDToolPatch = SchemaIDToolResource + "#/$defs/ToolPatch"
