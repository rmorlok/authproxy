package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type KeyPublicPrivate struct {
	PublicKey  KeyData `json:"public_key" yaml:"public_key"`
	PrivateKey KeyData `json:"private_key" yaml:"private_key"`
}

func (kpp *KeyPublicPrivate) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("key public/private expected a mapping node, got %s", KindToString(value.Kind))
	}

	var publicKey KeyData
	var privateKey KeyData

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "public_key":
			if publicKey, err = keyDataUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "private_key":
			if privateKey, err = keyDataUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType KeyPublicPrivate
	raw := (*RawType)(kpp)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.PublicKey = publicKey
	raw.PrivateKey = privateKey

	return nil
}

func (kpp *KeyPublicPrivate) CanSign() bool {
	return kpp.PrivateKey != nil
}

func (kpp *KeyPublicPrivate) CanVerifySignature() bool {
	return kpp.PublicKey != nil
}
