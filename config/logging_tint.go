package config

import (
	"github.com/lmittmann/tint"
	"log/slog"
	"sync"
	"time"
)

type LoggingConfigTint struct {
	Type       LoggingConfigType   `json:"type" yaml:"type"`
	To         LoggingConfigOutput `json:"to,omitempty" yaml:"to,omitempty"`
	Level      LoggingConfigLevel  `json:"level,omitempty" yaml:"level,omitempty"`
	Source     bool                `json:"source,omitempty" yaml:"source,omitempty"`
	NoColor    *bool               `json:"no_color,omitempty" yaml:"no_color,omitempty"`
	TimeFormat *string             `json:"time_format,omitempty" yaml:"time_format,omitempty"`
	once       sync.Once           `json:"-" yaml:"-"`
}

func (l *LoggingConfigTint) GetType() LoggingConfigType {
	return LoggingConfigTypeTint
}

func (l *LoggingConfigTint) GetRootLogger() *slog.Logger {
	var logger *slog.Logger

	noColor := false
	if l.NoColor != nil {
		noColor = *l.NoColor
	}

	timeFormat := time.Kitchen
	if l.TimeFormat != nil {
		timeFormat = *l.TimeFormat
	}

	l.once.Do(func() {
		handler := tint.NewHandler(l.To.Output(), &tint.Options{
			Level:      l.Level.Level(), // This configures minimum level
			AddSource:  l.Source,
			NoColor:    noColor,
			TimeFormat: timeFormat,
		})

		logger = slog.New(handler)
	})

	return logger
}
