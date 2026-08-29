package util

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// YamlBytesToJSON translates loaded YAML data to JSON as bytes.
func YamlBytesToJSON(yamlData []byte) ([]byte, error) {
	var v interface{}
	if err := yaml.Unmarshal(yamlData, &v); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	// YAML numbers default to int/float; ensure JSON-encodable
	j, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	return j, nil
}

// YamlDocumentEmpty reports whether the specified YAML document has no content.
func YamlDocumentEmpty(node *yaml.Node) bool {
	if node == nil || len(node.Content) == 0 {
		return true
	}

	value := node.Content[0]
	return value.Kind == yaml.ScalarNode && value.Tag == "!!null" && strings.TrimSpace(value.Value) == ""
}
