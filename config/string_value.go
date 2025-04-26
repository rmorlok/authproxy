package config

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
)

type StringValue interface {
	// HasValue checks if this value has data.
	HasValue(ctx context.Context) bool

	// GetValue retrieves the bytes of the key
	GetValue(ctx context.Context) (string, error)
}

func UnmarshallYamlStringValueString(data string) (StringValue, error) {
	return UnmarshallYamlStringValue([]byte(data))
}

func UnmarshallYamlStringValue(data []byte) (StringValue, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return stringValueUnmarshalYAML(rootNode.Content[0])
}

// stringValueUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func stringValueUnmarshalYAML(value *yaml.Node) (StringValue, error) {
	if value.Kind == yaml.ScalarNode {
		return &StringValueDirect{Value: value.Value}, nil
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("string value expected a scalar or mapping node, got %s", KindToString(value.Kind))
	}

	var keyData StringValue

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			keyData = &StringValueDirect{}
			break fieldLoop
		case "base64":
			keyData = &StringValueBase64{}
			break fieldLoop
		case "env_var":
			keyData = &StringValueEnvVar{}
			break fieldLoop
		case "env_var_base64":
			keyData = &StringValueEnvVarBase64{}
			break fieldLoop
		case "path":
			keyData = &StringValueFile{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return nil, fmt.Errorf("invalid structure for value type; does not match value, base64, env_var, file")
	}

	if err := value.Decode(keyData); err != nil {
		return nil, err
	}

	return keyData, nil
}
