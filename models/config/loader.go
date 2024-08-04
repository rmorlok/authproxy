package config

import "gopkg.in/yaml.v3"

func LoadConfig(in []byte) (*Root, error) {
	var root Root
	if err := yaml.Unmarshal(in, &root); err != nil {
		return nil, err
	}

	return &root, nil
}
