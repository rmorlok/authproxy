package common

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// RawJSON holds arbitrary JSON data that can be deserialized from both YAML and JSON.
// When unmarshaled from YAML, it converts the YAML structure to its JSON representation.
// When marshaled, it outputs raw JSON bytes.
type RawJSON json.RawMessage

// MarshalJSON returns the raw JSON bytes.
func (r RawJSON) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(r).MarshalJSON()
}

// UnmarshalJSON stores the raw JSON bytes.
func (r *RawJSON) UnmarshalJSON(data []byte) error {
	rm := json.RawMessage(data)
	*r = RawJSON(rm)
	return nil
}

// UnmarshalYAML converts YAML data to JSON bytes for storage.
func (r *RawJSON) UnmarshalYAML(value *yaml.Node) error {
	var raw interface{}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	*r = RawJSON(jsonBytes)
	return nil
}

// IsEmpty returns true if the raw JSON is nil or empty.
func (r RawJSON) IsEmpty() bool {
	return len(r) == 0
}
