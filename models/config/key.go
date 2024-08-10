package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Key interface {
	// CanSign checks if the key can sign requests (either private key is present or shared key)
	CanSign() bool
	// CanVerifySignature checks if the key can be used to verify the signature of something (public key is present or shared key)
	CanVerifySignature() bool
}

func UnmarshallYamlKeyString(data string) (Key, error) {
	return UnmarshallYamlKey([]byte(data))
}

func UnmarshallYamlKey(data []byte) (Key, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return keyUnmarshalYAML(rootNode.Content[0])
}

// keyUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func keyUnmarshalYAML(value *yaml.Node) (Key, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var key Key

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]

		switch keyNode.Value {
		case "public_key":
			key = &KeyPublicPrivate{}
			break fieldLoop
		case "private_key":
			key = &KeyPublicPrivate{}
			break fieldLoop
		case "shared_key":
			key = &KeyShared{}
			break fieldLoop
		}
	}

	if key == nil {
		return nil, fmt.Errorf("invalid structure for key type; does not match value, public_key/private_key or shared_key")
	}

	if err := value.Decode(key); err != nil {
		return nil, err
	}

	return key, nil
}
