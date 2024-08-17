package config

import "gopkg.in/yaml.v3"

func marshalToYamlString(v interface{}) (string, error) {
	bytes, err := yaml.Marshal(v)
	return string(bytes), err
}

func mustMarshalToYamlString(v interface{}) string {
	if s, err := marshalToYamlString(v); err != nil {
		panic(err)
	} else {
		return s
	}
}
