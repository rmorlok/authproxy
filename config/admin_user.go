package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AdminUser struct {
	Username string `json:"username" yaml:"username"`
	Key      Key    `json:"key" yaml:"key"`
}

func (au *AdminUser) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("admin user expected a mapping node, got %s", KindToString(value.Kind))
	}

	var key Key

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "key":
			if key, err = keyUnmarshalYAML(valueNode); err != nil {
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
	type RawType AdminUser
	raw := (*RawType)(au)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Key = key

	return nil
}

func UnmarshallYamlAdminUserString(data string) (*AdminUser, error) {
	return UnmarshallYamlAdminUser([]byte(data))
}

func UnmarshallYamlAdminUser(data []byte) (*AdminUser, error) {
	var au AdminUser
	if err := yaml.Unmarshal(data, &au); err != nil {
		return nil, err
	}

	return &au, nil
}
