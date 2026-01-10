package config

import (
	"log/slog"
)

type LoggingConfigJson struct {
	Type   LoggingConfigType   `json:"type" yaml:"type"`
	To     LoggingConfigOutput `json:"to,omitempty" yaml:"to,omitempty"`
	Level  LoggingConfigLevel  `json:"level,omitempty" yaml:"level,omitempty"`
	Source bool                `json:"source,omitempty" yaml:"source,omitempty"`
}

func (l *LoggingConfigJson) GetType() LoggingConfigType {
	return LoggingConfigTypeJson
}

func (l *LoggingConfigJson) GetRootLogger() *slog.Logger {
	handler := slog.NewJSONHandler(l.To.Output(), &slog.HandlerOptions{
		Level:     l.Level.Level(), // This configures minimum level
		AddSource: l.Source,
	})

	return slog.New(handler)
}
