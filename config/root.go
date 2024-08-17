package config

import (
	"gopkg.in/yaml.v3"
)

type Root struct {
	AdminApi     ApiHost       `json:"admin_api" yaml:"admin_api"`
	SystemAuth   SystemAuth    `json:"system_auth" yaml:"system_auth"`
	Integrations []Integration `json:"integrations" yaml:"integrations"`
}

func UnmarshallYamlRootString(data string) (*Root, error) {
	return UnmarshallYamlRoot([]byte(data))
}

func UnmarshallYamlRoot(data []byte) (*Root, error) {
	var root Root
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	return &root, nil
}
