package config

import (
	"encoding/json"
	"fmt"
)

func (d *Database) MarshalJSON() ([]byte, error) {
	if d == nil || d.InnerVal == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(d.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (d *Database) UnmarshalJSON(data []byte) error {
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal database: %v", err)
	}

	var t DatabaseImpl

	if provider, ok := valueMap["provider"]; ok {
		switch DatabaseProvider(fmt.Sprintf("%v", provider)) {
		case DatabaseProviderSqlite:
			t = &DatabaseSqlite{Provider: DatabaseProviderSqlite}
		case DatabaseProviderPostgres:
			t = &DatabasePostgres{Provider: DatabaseProviderPostgres}
		case DatabaseProviderClickhouse:
			t = &DatabaseClickhouse{Provider: DatabaseProviderClickhouse}
		default:
			return fmt.Errorf("unknown database provider %v", provider)
		}
	} else {
		return fmt.Errorf("invalid structure for database; missing provider field")
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	d.InnerVal = t
	return nil
}
