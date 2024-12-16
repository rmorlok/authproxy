package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

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

func KindToString(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "DocumentNode"
	case yaml.SequenceNode:
		return "SequenceNode"
	case yaml.MappingNode:
		return "MappingNode"
	case yaml.ScalarNode:
		return "ScalarNode"
	case yaml.AliasNode:
		return "AliasNode"
	default:
		return fmt.Sprintf("unknown (%d)", k)
	}
}
