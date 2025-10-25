package config

import (
	"embed"
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

//go:embed schema.json
var schema embed.FS

// readSchemaBytes reads the schema.json file from the embedded filesystem.
func readSchemaBytes() ([]byte, error) {
	return schema.ReadFile("schema.json")
}

// yamlBytesToJSON translates loaded YAML data to JSON as bytes.
func yamlBytesToJSON(yamlData []byte) ([]byte, error) {
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
