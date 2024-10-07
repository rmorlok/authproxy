package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type AdminUsersList []*AdminUser

func (aul AdminUsersList) All() []*AdminUser {
	return aul
}

func (aul AdminUsersList) GetByUsername(username string) (*AdminUser, bool) {
	for _, aulUser := range aul {
		if aulUser.Username == username {
			return aulUser, true
		}
	}

	return nil, false
}

func UnmarshallYamlAdminUsersListString(data string) (AdminUsersList, error) {
	return UnmarshallYamlAdminUsersList([]byte(data))
}

func UnmarshallYamlAdminUsersList(data []byte) (AdminUsersList, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return adminUsersListUnmarshalYAML(rootNode.Content[0])
}

// adminUsersUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func adminUsersListUnmarshalYAML(value *yaml.Node) (AdminUsersList, error) {
	// Ensure the node is a sequence node
	if value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("expected a sequence node, got %v", value.Kind)
	}

	var results []*AdminUser
	for _, childNode := range value.Content {
		var adminUser AdminUser
		if err := childNode.Decode(&adminUser); err != nil {
			return nil, err
		}
		results = append(results, &adminUser)
	}

	return AdminUsersList(results), nil
}
