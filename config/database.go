package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

type DatabaseProvider string

const (
	DatabaseProviderSqlite DatabaseProvider = "sqlite"
)

type Database interface {
	GetProvider() DatabaseProvider
}

func UnmarshallYamlDatabaseString(data string) (Database, error) {
	return UnmarshallYamlDatabase([]byte(data))
}

func UnmarshallYamlDatabase(data []byte) (Database, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return databaseUnmarshalYAML(rootNode.Content[0])
}

// databaseUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func databaseUnmarshalYAML(value *yaml.Node) (Database, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("database expected a mapping node, got %s", KindToString(value.Kind))
	}

	var database Database

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "provider":
			switch DatabaseProvider(valueNode.Value) {
			case DatabaseProviderSqlite:
				database = &DatabaseSqlite{}
				break fieldLoop
			default:
				return nil, fmt.Errorf("unknown database provider %v", valueNode.Value)
			}

		}
	}

	if database == nil {
		return nil, fmt.Errorf("invalid structure for database; missing provider field")
	}

	if err := value.Decode(database); err != nil {
		return nil, err
	}

	return database, nil
}
