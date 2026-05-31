package json_schema

const SchemaIdJSONSchema = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/common/json_schema/schema.json"

// Property describes a single JSON Schema property emitted in a setup form.
type Property struct {
	Type      string `json:"type" yaml:"type"`
	Title     string `json:"title,omitempty" yaml:"title,omitempty"`
	MinLength int    `json:"minLength,omitempty" yaml:"minLength,omitempty"`
}

// Schema describes the JSON Schema object emitted for a setup form.
type Schema struct {
	Type                 string              `json:"type" yaml:"type"`
	Required             []string            `json:"required" yaml:"required"`
	Properties           map[string]Property `json:"properties" yaml:"properties"`
	AdditionalProperties bool                `json:"additionalProperties" yaml:"additionalProperties"`
}
