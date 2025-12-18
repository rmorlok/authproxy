package aplog

import (
	"log/slog"
	"sync"
)

var defaultOnce = sync.Once{}

func SetDefaultLog(logger *slog.Logger) {
	if logger == nil {
		return
	}

	defaultOnce.Do(func() {
		slog.SetDefault(logger)
	})
}

type HasLogger interface {
	Logger() *slog.Logger
}

// LoggerOrDefault returns logger from first option that implements HasLogger interface, or the default logger.
func LoggerOrDefault(options ...interface{}) *slog.Logger {
	for _, opt := range options {
		if l, ok := opt.(HasLogger); ok {
			return l.Logger()
		}
	}

	return slog.Default()
}
