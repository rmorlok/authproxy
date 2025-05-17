package common

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

func MarshalToYamlString(v interface{}) (string, error) {
	bytes, err := yaml.Marshal(v)
	return string(bytes), err
}

func MustMarshalToYamlString(v interface{}) string {
	if s, err := MarshalToYamlString(v); err != nil {
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