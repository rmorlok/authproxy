package config

import (
	"encoding/json"
	"fmt"
)

func (l *LoggingConfig) MarshalJSON() ([]byte, error) {
	if l == nil || l.InnerVal == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(l.InnerVal)
}

// UnmarshalJSON handles unmarshalling from JSON while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (l *LoggingConfig) UnmarshalJSON(data []byte) error {
	var valueMap map[string]interface{}
	if err := json.Unmarshal(data, &valueMap); err != nil {
		return fmt.Errorf("failed to unmarshal logging: %v", err)
	}

	var t LoggingImpl

	if typ, ok := valueMap["type"]; ok {
		switch LoggingConfigType(fmt.Sprintf("%v", typ)) {
		case LoggingConfigTypeText:
			t = &LoggingConfigText{}
		case LoggingConfigTypeJson:
			t = &LoggingConfigJson{}
		case LoggingConfigTypeTint:
			t = &LoggingConfigTint{}
		case LoggingConfigTypeNone:
			t = &LoggingConfigNone{}
		default:
			return fmt.Errorf("unknown logging type %v", typ)
		}
	} else {
		return fmt.Errorf("invalid structure for logging; missing type field")
	}

	if err := json.Unmarshal(data, t); err != nil {
		return err
	}

	l.InnerVal = t
	return nil
}
