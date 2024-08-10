package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type SystemAuth struct {
	JwtSigningKey Key `json:"jwt_signing_key" yaml:"jwt_signing_key"`
}

func (sa *SystemAuth) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node, got %v", value.Kind)
	}

	var jwtSigngingKey Key

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "jwt_signing_key":
			if jwtSigngingKey, err = keyUnmarshalYAML(valueNode); err != nil {
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
	type RawType SystemAuth
	raw := (*RawType)(sa)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.JwtSigningKey = jwtSigngingKey

	return nil
}
