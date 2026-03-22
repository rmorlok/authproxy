package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rmorlok/authproxy/internal/schema"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

func LoadConfig(path string) (C, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	schema, err := schema.CompileSchema(schema.SchemaIdConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to read config schema: %w", err)
	}

	configJsonBytes, err := util.YamlBytesToJSON(content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON for config schema validation: %w", err)
	}

	var configAsParsedJson interface{}
	if err := json.Unmarshal(configJsonBytes, &configAsParsedJson); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config JSON for config schema validation: %w", err)
	}

	if err := schema.Validate(configAsParsedJson); err != nil {
		return nil, fmt.Errorf("config schema validation failed: %w", err)
	}

	var root sconfig.Root
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, err
	}

	return &config{root: &root}, nil
}

func FromRoot(root *sconfig.Root) C {
	return &config{root: root}
}
