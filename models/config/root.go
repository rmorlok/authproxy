package config

import "gopkg.in/yaml.v3"

type Root struct {
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
