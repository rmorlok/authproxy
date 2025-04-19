package config

import (
	"log/slog"
)

type LoggingConfigText struct {
	Type   LoggingConfigType   `json:"type" yaml:"type"`
	To     LoggingConfigOutput `json:"to,omitempty" yaml:"to,omitempty"`
	Level  LoggingConfigLevel  `json:"level,omitempty" yaml:"level,omitempty"`
	Source bool                `json:"source,omitempty" yaml:"source,omitempty"`
}

func (l *LoggingConfigText) GetType() LoggingConfigType {
	return LoggingConfigTypeText
}

func (l *LoggingConfigText) GetRootLogger() *slog.Logger {
	handler := slog.NewTextHandler(l.To.Output(), &slog.HandlerOptions{
		Level:     l.Level.Level(), // This configures minimum level
		AddSource: l.Source,
	})

	return slog.New(handler)
}
