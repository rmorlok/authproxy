package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type KeyShared struct {
	SharedKey KeyData `json:"shared_key" yaml:"shared_key"`
}

func (ks *KeyShared) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var sharedKey KeyData

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "shared_key":
			if sharedKey, err = keyDataUnmarshalYAML(valueNode); err != nil {
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
	type RawType KeyShared
	raw := (*RawType)(ks)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.SharedKey = sharedKey

	return nil
}

func (ks *KeyShared) CanSign() bool {
	return true
}

func (ks *KeyShared) CanVerifySignature() bool {
	return true
}
