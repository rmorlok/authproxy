package config

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/schema"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
)

func LoadConfig(path string) (C, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	schema, err := schema.CompileSchema(schema.SchemaIdConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config schema")
	}

	configJsonBytes, err := util.YamlBytesToJSON(content)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert YAML to JSON for config schema validation")
	}

	var configAsParsedJson interface{}
	if err := json.Unmarshal(configJsonBytes, &configAsParsedJson); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config JSON for config schema validation")
	}

	if err := schema.Validate(configAsParsedJson); err != nil {
		return nil, errors.Wrap(err, "config schema validation failed")
	}

	root, err := sconfig.UnmarshallYamlRoot(content)
	if err != nil {
		return nil, err
	}

	return &config{root: root}, nil
}

func FromRoot(root *sconfig.Root) C {
	return &config{root: root}
}
