package connectors

import (
	"gopkg.in/yaml.v3"
)

type Scope struct {
	Id       string `json:"id" yaml:"id"`
	Required bool   `json:"required" yaml:"required"`
	Reason   string `json:"reason" yaml:"reason"`
}

func UnmarshallYamlScopeString(data string) (*Scope, error) {
	return UnmarshallYamlScope([]byte(data))
}

func UnmarshallYamlScope(data []byte) (*Scope, error) {
	var scope Scope
	if err := yaml.Unmarshal(data, &scope); err != nil {
		return nil, err
	}

	return &scope, nil
}

func (s *Scope) UnmarshalYAML(value *yaml.Node) error {
	type Raw Scope // Type alias to avoid recursion
	raw := Raw{
		Required: true, // Default to required if not specified
	}

	if err := value.Decode(&raw); err != nil {
		return err
	}

	*s = Scope(raw)

	return nil
}