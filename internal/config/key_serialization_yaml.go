package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (k *Key) MarshalYAML() (interface{}, error) {
	if k == nil || k.InnerVal == nil {
		return nil, nil
	}

	return k.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (k *Key) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("key expected a mapping node, got %s", KindToString(value.Kind))
	}

	var key KeyType

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
		return fmt.Errorf("invalid structure for key type; does not match value, public_key/private_key or shared_key")
	}

	if err := value.Decode(key); err != nil {
		return err
	}

	k.InnerVal = key

	return nil
}
