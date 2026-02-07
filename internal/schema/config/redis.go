package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type RedisProvider string

const (
	RedisProviderMiniredis RedisProvider = "miniredis"
	RedisProviderRedis     RedisProvider = "redis"
)

type Redis interface {
	GetProvider() RedisProvider
}

func UnmarshallYamlRedisString(data string) (Redis, error) {
	return UnmarshallYamlRedis([]byte(data))
}

func UnmarshallYamlRedis(data []byte) (Redis, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return redisUnmarshalYAML(rootNode.Content[0])
}

// RedisUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func redisUnmarshalYAML(value *yaml.Node) (Redis, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Redis expected a mapping node, got %s", KindToString(value.Kind))
	}

	var redis Redis = &RedisReal{}

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
				return nil, fmt.Errorf("unknown redis provider %v", valueNode.Value)
			}

		}
	}

	if err := value.Decode(redis); err != nil {
		return nil, err
	}

	return redis, nil
}
