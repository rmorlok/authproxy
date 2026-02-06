package config

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
)

type LoggingConfigType string

const (
	LoggingConfigTypeText LoggingConfigType = "text"
	LoggingConfigTypeJson LoggingConfigType = "json"
	LoggingConfigTypeTint LoggingConfigType = "tint"
	LoggingConfigTypeNone LoggingConfigType = "none"
)

type LoggingConfigLevel string

const (
	LevelDebug LoggingConfigLevel = "debug"
	LevelInfo  LoggingConfigLevel = "info"
	LevelWarn  LoggingConfigLevel = "warn"
	LevelError LoggingConfigLevel = "error"
)

func (l LoggingConfigLevel) String() string {
	return string(l)
}
func (l LoggingConfigLevel) Level() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type LoggingConfigOutput string

const (
	OutputStdout LoggingConfigOutput = "stdout"
	OutputStderr LoggingConfigOutput = "stderr"
)

func (l LoggingConfigOutput) Output() *os.File {
	switch l {
	case OutputStdout:
		return os.Stdout
	case OutputStderr:
		return os.Stderr
	default:
		return os.Stderr
	}
}

type LoggingConfig interface {
	GetRootLogger() *slog.Logger
	GetType() LoggingConfigType
}

func UnmarshallYamlLoggingString(data string) (LoggingConfig, error) {
	return UnmarshallYamlLogging([]byte(data))
}

func UnmarshallYamlLogging(data []byte) (LoggingConfig, error) {
	var rootNode yaml.Node

	if err := yaml.Unmarshal(data, &rootNode); err != nil {
		return nil, err
	}

	return loggingUnmarshalYAML(rootNode.Content[0])
}

// loggingUnmarshalYAML handles unmarshalling from YAML while allowing us to make decisions
// about how the data is unmarshalled based on the concrete type being represented
func loggingUnmarshalYAML(value *yaml.Node) (LoggingConfig, error) {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("logger expected a mapping node, got %s", KindToString(value.Kind))
	}

	var loggigConfig LoggingConfig

fieldLoop:
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		switch keyNode.Value {
		case "type":
			switch LoggingConfigType(valueNode.Value) {
			case LoggingConfigTypeText:
				loggigConfig = &LoggingConfigText{Type: LoggingConfigTypeText}
				break fieldLoop
			case LoggingConfigTypeJson:
				loggigConfig = &LoggingConfigJson{Type: LoggingConfigTypeJson}
				break fieldLoop
			case LoggingConfigTypeTint:
				loggigConfig = &LoggingConfigTint{Type: LoggingConfigTypeTint}
				break fieldLoop
			default:
				return nil, fmt.Errorf("unknown logging provider type %v", valueNode.Value)
			}

		}
	}

	if loggigConfig == nil {
		return nil, fmt.Errorf("invalid structure for logging; missing type field")
	}

	if err := value.Decode(loggigConfig); err != nil {
		return nil, err
	}

	return loggigConfig, nil
}
