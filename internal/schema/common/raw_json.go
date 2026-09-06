package common

import (
	"encoding/json"
	"fmt"

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

// MarshalYAML decodes the JSON representation before handing it to the YAML
// encoder so objects and arrays remain structured YAML values rather than
// being emitted as an encoded byte slice.
func (r RawJSON) MarshalYAML() (interface{}, error) {
	if r == nil {
		return nil, nil
	}

	var value interface{}
	if err := json.Unmarshal(r, &value); err != nil {
		return nil, fmt.Errorf("invalid raw JSON: %w", err)
	}
	return value, nil
}

// IsEmpty returns true if the raw JSON is nil or empty.
func (r RawJSON) IsEmpty() bool {
	return len(r) == 0
}
