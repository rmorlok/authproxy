package util

import (
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// YamlBytesToJSON translates loaded YAML data to JSON as bytes.
func YamlBytesToJSON(yamlData []byte) ([]byte, error) {
	var v interface{}
	if err := yaml.Unmarshal(yamlData, &v); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal YAML")
	}
	// YAML numbers default to int/float; ensure JSON-encodable
	j, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal to JSON")
	}

	return j, nil
}
