package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func (l *LoggingConfig) MarshalYAML() (interface{}, error) {
	if l.InnerVal == nil {
		return nil, nil
	}
	return l.InnerVal, nil
}

// UnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func (l *LoggingConfig) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("logger expected a mapping node, got %s", KindToString(value.Kind))
	}

	var loggingConfig LoggingImpl

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "type":
			switch LoggingConfigType(valueNode.Value) {
			case LoggingConfigTypeText:
				loggingConfig = &LoggingConfigText{Type: LoggingConfigTypeText}
				break fieldLoop
			case LoggingConfigTypeJson:
				loggingConfig = &LoggingConfigJson{Type: LoggingConfigTypeJson}
				break fieldLoop
			case LoggingConfigTypeTint:
				loggingConfig = &LoggingConfigTint{Type: LoggingConfigTypeTint}
				break fieldLoop
			case LoggingConfigTypeNone:
				loggingConfig = &LoggingConfigNone{Type: LoggingConfigTypeNone}
				break fieldLoop
			default:
				return fmt.Errorf("unknown logging type %v", valueNode.Value)
			}
		}
	}

	if loggingConfig == nil {
		return fmt.Errorf("invalid structure for logging; missing type field")
	}

	if err := value.Decode(loggingConfig); err != nil {
		return err
	}

	l.InnerVal = loggingConfig
	return nil
}
