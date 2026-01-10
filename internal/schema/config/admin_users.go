package config

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

type AdminUsersType interface {
	All() []*AdminUser
	GetByUsername(username string) (*AdminUser, bool)
	GetByJwtSubject(subject string) (*AdminUser, bool)
}

type AdminUsers struct {
	InnerVal AdminUsersType `json:"-" yaml:"-"`
}

func (au *AdminUsers) All() []*AdminUser {
	return au.InnerVal.All()
}

func (au *AdminUsers) GetByUsername(username string) (*AdminUser, bool) {
	return au.InnerVal.GetByUsername(username)
}

func (au *AdminUsers) GetByJwtSubject(subject string) (*AdminUser, bool) {
	return au.InnerVal.GetByJwtSubject(subject)
}

func (au *AdminUsers) MarshalJSON() ([]byte, error) {
	if au.InnerVal == nil {
		return json.Marshal(nil)
	}

	return json.Marshal(au.InnerVal)
}

func (au *AdminUsers) UnmarshalJSON(data []byte) error {
	if len(data) >= 2 && data[0] == '[' && data[len(data)-1] == ']' {
		var adminUsersList AdminUsersList
		err := json.Unmarshal(data, &adminUsersList)
		au.InnerVal = adminUsersList
		return err
	}

	// If it's not a string, it should be an object
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal string value: %v", err)
	}

	var t AdminUsersType

	if _, ok := valueMap["keys_path"]; ok {
		t = &AdminUsersExternalSource{}
	} else {
		return fmt.Errorf("invalid structure for admin users; must be list or have keys_path")
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	au.InnerVal = t

	return nil
}

func (au *AdminUsers) MarshalYAML() (interface{}, error) {
	if au.InnerVal == nil {
		return nil, nil
	}

	return au.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (au *AdminUsers) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.SequenceNode {
		var adminUsersList AdminUsersList
		err := value.Decode(&adminUsersList)
		au.InnerVal = adminUsersList
		return err
	}

	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("admin users expected a sequence node or mapping node, got %s", KindToString(value.Kind))
	}

	var adminUsersExternalSource AdminUsersExternalSource
	if err := value.Decode(&adminUsersExternalSource); err != nil {
		return err
	}

	au.InnerVal = &adminUsersExternalSource
	return nil
}

var _ AdminUsersType = (*AdminUsers)(nil)
