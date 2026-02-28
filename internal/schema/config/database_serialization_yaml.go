package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (d *Database) MarshalYAML() (interface{}, error) {
	if d.InnerVal == nil {
		return nil, nil
	}
	return d.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (d *Database) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("database expected a mapping node, got %s", KindToString(value.Kind))
	}

	var database DatabaseImpl

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "provider":
			switch DatabaseProvider(valueNode.Value) {
			case DatabaseProviderSqlite:
				database = &DatabaseSqlite{Provider: DatabaseProviderSqlite}
				break fieldLoop
			case DatabaseProviderPostgres:
				database = &DatabasePostgres{Provider: DatabaseProviderPostgres}
				break fieldLoop
			case DatabaseProviderClickhouse:
				database = &DatabaseClickhouse{Provider: DatabaseProviderClickhouse}
				break fieldLoop
			default:
				return fmt.Errorf("unknown database provider %v", valueNode.Value)
			}
		}
	}

	if database == nil {
		return fmt.Errorf("invalid structure for database; missing provider field")
	}

	if err := value.Decode(database); err != nil {
		return err
	}

	d.InnerVal = database
	return nil
}
