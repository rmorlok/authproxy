package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type Connector struct {
	Id          string `json:"id" yaml:"id"`
	Version     uint64 `json:"version" yaml:"version"`
	DisplayName string `json:"display_name" yaml:"display_name"`
	Logo        Image  `json:"logo" yaml:"logo"`
	Description string `json:"description" yaml:"description"`
	Auth        Auth   `json:"auth" yaml:"auth"`
}

func (c *Connector) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("connector expected a mapping node, got %s", KindToString(value.Kind))
	}

	var image Image
	var auth Auth

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "logo":
			if image, err = imageUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "auth":
			if auth, err = authUnmarshalYAML(valueNode); err != nil {
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
	type RawType Connector
	raw := (*RawType)(c)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Logo = image
	raw.Auth = auth

	return nil
}
