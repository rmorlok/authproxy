package config

import (
	"fmt"
	"github.com/rmorlok/authproxy/context"
	"gopkg.in/yaml.v3"
)

type KeyData interface {
	// HasData checks if this value has data.
	HasData(ctx context.Context) bool

	// GetData retrieves the bytes of the key
	GetData(ctx context.Context) ([]byte, error)
}

func UnmarshallYamlKeyDataString(data string) (KeyData, error) {
	return UnmarshallYamlKeyData([]byte(data))
}

func UnmarshallYamlKeyData(data []byte) (KeyData, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return keyDataUnmarshalYAML(rootNode.Content[0])
}

// keyUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func keyDataUnmarshalYAML(value *yaml.Node) (KeyData, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("key data expected a mapping node, got %s", KindToString(value.Kind))
	}

	var keyData KeyData

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "value":
			keyData = &KeyDataValue{}
			break fieldLoop
		case "base64":
			keyData = &KeyDataBase64Val{}
			break fieldLoop
		case "env_var":
			keyData = &KeyDataEnvVar{}
			break fieldLoop
		case "path":
			keyData = &KeyDataFile{}
			break fieldLoop
		case "random":
			keyData = &KeyDataRandomBytes{}
			break fieldLoop
		}
	}

	if keyData == nil {
		return nil, fmt.Errorf("invalid structure for key data type; does not match value, base64, env_var, file")
	}

	if err := value.Decode(keyData); err != nil {
		return nil, err
	}

	return keyData, nil
}
