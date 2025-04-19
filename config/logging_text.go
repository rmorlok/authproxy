package config

import (
	"log/slog"
	"sync"
)

type LoggingConfigText struct {
	Type   LoggingConfigType   `json:"type" yaml:"type"`
	To     LoggingConfigOutput `json:"to,omitempty" yaml:"to,omitempty"`
	Level  LoggingConfigLevel  `json:"level,omitempty" yaml:"level,omitempty"`
	Source bool                `json:"source,omitempty" yaml:"source,omitempty"`
	once   sync.Once           `json:"-" yaml:"-"`
}

func (l *LoggingConfigText) GetType() LoggingConfigType {
	return LoggingConfigTypeText
}

func (l *LoggingConfigText) GetRootLogger() *slog.Logger {
	var logger *slog.Logger

	l.once.Do(func() {
		handler := slog.NewTextHandler(l.To.Output(), &slog.HandlerOptions{
			Level:     l.Level.Level(), // This configures minimum level
			AddSource: l.Source,
		})

		logger = slog.New(handler)
	})

	return logger
}
