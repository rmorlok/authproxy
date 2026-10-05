package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

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

// MarshalYAML emits structured YAML without decoding numbers through float64.
// Keeping numeric nodes intact avoids rounding large integer schema constraints
// or payload values merely by exporting a resource as YAML. This preserves
// export precision; UnmarshalYAML still uses yaml.v3's native numeric decoding.
func (r RawJSON) MarshalYAML() (interface{}, error) {
	if r == nil {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(r))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid raw JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("invalid raw JSON: expected exactly one JSON value")
	}
	return rawJSONYAMLNode(value)
}

// rawJSONYAMLNode converts values from a UseNumber decoder to YAML nodes.
// Explicit numeric tags preserve both the original number text and its type,
// even when yaml.v3's implicit resolver would treat an overflow as a string.
// Sorted object keys make exports deterministic; the encoder chooses quoting
// for ordinary scalars and readable block formatting for containers.
func rawJSONYAMLNode(value any) (*yaml.Node, error) {
	switch value := value.(type) {
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(value.String(), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value.String()}, nil
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child, err := rawJSONYAMLNode(value[key])
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range value {
			child, err := rawJSONYAMLNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	default:
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		return &node, nil
	}
}

// IsEmpty returns true if the raw JSON is nil or empty.
func (r RawJSON) IsEmpty() bool {
	return len(r) == 0
}
