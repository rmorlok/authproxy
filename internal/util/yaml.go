package util

import (
	"encoding/json"
	"fmt"

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
