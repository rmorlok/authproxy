package config

import (
	"fmt"
	"github.com/rmorlok/authproxy/config/common"
	"gopkg.in/yaml.v3"
)

type Marketplace struct {
	BaseUrl StringValue `json:"base_url,omitempty" yaml:"base_url,omitempty"`
}

func (s *Marketplace) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("marketplace expected a mapping node, got %s", KindToString(value.Kind))
	}

	var baseUrlVal StringValue = nil

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "base_url":
			if baseUrlVal, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
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
	type RawType Marketplace
	raw := (*RawType)(s)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.BaseUrl = baseUrlVal

	return nil
}
