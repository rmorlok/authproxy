package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (r *Redis) MarshalYAML() (interface{}, error) {
	if r.InnerVal == nil {
		return nil, nil
	}
	return r.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (r *Redis) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("Redis expected a mapping node, got %s", KindToString(value.Kind))
	}

	var redis RedisImpl = &RedisReal{}

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "provider":
			switch RedisProvider(valueNode.Value) {
			case RedisProviderMiniredis:
				redis = &RedisMiniredis{
					Provider: RedisProviderMiniredis,
				}
				break fieldLoop
			case RedisProviderRedis:
				redis = &RedisReal{
					Provider: RedisProviderRedis,
				}
				break fieldLoop
			default:
				return fmt.Errorf("unknown redis provider %v", valueNode.Value)
			}
		}
	}

	if err := value.Decode(redis); err != nil {
		return err
	}

	r.InnerVal = redis
	return nil
}
