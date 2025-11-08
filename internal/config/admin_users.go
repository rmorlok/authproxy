package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AdminUsers interface {
	All() []*AdminUser
	GetByUsername(username string) (*AdminUser, bool)
	GetByJwtSubject(subject string) (*AdminUser, bool)
}

func UnmarshallYamlAdminUsersString(data string) (AdminUsers, error) {
	return UnmarshallYamlAdminUsers([]byte(data))
}

func UnmarshallYamlAdminUsers(data []byte) (AdminUsers, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return adminUsersUnmarshalYAML(rootNode.Content[0])
}

// adminUsersUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func adminUsersUnmarshalYAML(value *yaml.Node) (AdminUsers, error) {
	if value.Kind == yaml.SequenceNode {
		return adminUsersListUnmarshalYAML(value)
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("admin users expected a sequence node or mapping node, got %s", KindToString(value.Kind))
	}

	var adminUsersExternalSource AdminUsersExternalSource
	if err := value.Decode(&adminUsersExternalSource); err != nil {
		return nil, err
	}

	return &adminUsersExternalSource, nil
}
