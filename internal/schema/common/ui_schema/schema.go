package ui_schema

const SchemaIdUISchema = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/common/ui_schema/schema.json"

// Control describes a JSON Forms control element rendered by the setup UI.
type Control struct {
	Type    string            `json:"type" yaml:"type"`
	Scope   string            `json:"scope" yaml:"scope"`
	Options map[string]string `json:"options,omitempty" yaml:"options,omitempty"`
}

// Schema describes the JSON Forms UI schema emitted for a setup form.
type Schema struct {
	Type     string    `json:"type" yaml:"type"`
	Elements []Control `json:"elements" yaml:"elements"`
}
