package worker

import (
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

type asyncLogger struct {
	inner *slog.Logger
}

func (l *asyncLogger) Debug(args ...interface{}) {
	l.inner.Debug(fmt.Sprint(args...))
}

func (l *asyncLogger) Info(args ...interface{}) {
	l.inner.Info(fmt.Sprint(args...))
}

func (l *asyncLogger) Warn(args ...interface{}) {
	l.inner.Warn(fmt.Sprint(args...))
}

func (l *asyncLogger) Error(args ...interface{}) {
	l.inner.Error(fmt.Sprint(args...))
}

func (l *asyncLogger) Fatal(args ...interface{}) {
	l.inner.Error(fmt.Sprint(args...))
	panic(fmt.Sprint(args...))
}

var _ asynq.Logger = (*asyncLogger)(nil)
