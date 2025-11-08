package config

import (
	"io"
	"log/slog"
)

type LoggingConfigNone struct {
	Type LoggingConfigType `json:"type" yaml:"type"`
}

func (l *LoggingConfigNone) GetType() LoggingConfigType {
	return LoggingConfigTypeNone
}

func (l *LoggingConfigNone) GetRootLogger() *slog.Logger {
	handler := slog.NewJSONHandler(io.Discard, nil)
	return slog.New(handler)
}
