package config

import (
	"log/slog"
	"time"

	"github.com/lmittmann/tint"
)

type LoggingConfigTint struct {
	Type       LoggingConfigType   `json:"type" yaml:"type"`
	To         LoggingConfigOutput `json:"to,omitempty" yaml:"to,omitempty"`
	Level      LoggingConfigLevel  `json:"level,omitempty" yaml:"level,omitempty"`
	Source     bool                `json:"source,omitempty" yaml:"source,omitempty"`
	NoColor    *bool               `json:"no_color,omitempty" yaml:"no_color,omitempty"`
	TimeFormat *string             `json:"time_format,omitempty" yaml:"time_format,omitempty"`
}

func (l *LoggingConfigTint) GetType() LoggingConfigType {
	return LoggingConfigTypeTint
}

func (l *LoggingConfigTint) GetRootLogger() *slog.Logger {
	noColor := false
	if l.NoColor != nil {
		noColor = *l.NoColor
	}

	timeFormat := time.Kitchen
	if l.TimeFormat != nil {
		timeFormat = *l.TimeFormat
	}

	handler := tint.NewHandler(l.To.Output(), &tint.Options{
		Level:      l.Level.Level(), // This configures minimum level
		AddSource:  l.Source,
		NoColor:    noColor,
		TimeFormat: timeFormat,
	})

	return slog.New(handler)
}
