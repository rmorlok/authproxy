package config

import (
	"log/slog"
	"os"
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

// LoggingImpl is the interface implemented by concrete logging configurations.
type LoggingImpl interface {
	GetRootLogger() *slog.Logger
	GetType() LoggingConfigType
}

// LoggingConfig is the holder for a LoggingImpl instance.
type LoggingConfig struct {
	InnerVal LoggingImpl `json:"-" yaml:"-"`
}

func (l *LoggingConfig) GetRootLogger() *slog.Logger {
	if l == nil || l.InnerVal == nil {
		return (&LoggingConfigNone{Type: LoggingConfigTypeNone}).GetRootLogger()
	}
	return l.InnerVal.GetRootLogger()
}

func (l *LoggingConfig) GetType() LoggingConfigType {
	if l == nil || l.InnerVal == nil {
		return LoggingConfigTypeNone
	}
	return l.InnerVal.GetType()
}

var _ LoggingImpl = (*LoggingConfig)(nil)
