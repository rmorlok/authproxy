package config

import (
	"io"
	"log/slog"
	"sync"
)

type LoggingConfigNone struct {
	Type LoggingConfigType `json:"type" yaml:"type"`
	once sync.Once         `json:"-" yaml:"-"`
}

func (l *LoggingConfigNone) GetType() LoggingConfigType {
	return LoggingConfigTypeNone
}

func (l *LoggingConfigNone) GetRootLogger() *slog.Logger {
	var logger *slog.Logger

	l.once.Do(func() {
		handler := slog.NewJSONHandler(io.Discard, nil)
		logger = slog.New(handler)
	})

	return logger
}
