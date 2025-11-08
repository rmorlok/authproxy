package database

import (
	"context"
	"fmt"
	"github.com/rmorlok/authproxy/internal/apctx"
	glogger "gorm.io/gorm/logger"
	"log/slog"
	"time"
)

type logger struct {
	inner *slog.Logger
}

func (l *logger) Info(ctx context.Context, msg string, args ...interface{}) {
	l.inner.Info(fmt.Sprintf(msg, args...))
}

func (l *logger) Warn(ctx context.Context, msg string, args ...interface{}) {
	l.inner.Warn(fmt.Sprintf(msg, args...))
}

func (l *logger) Error(ctx context.Context, msg string, args ...interface{}) {
	l.inner.Error(fmt.Sprintf(msg, args...))
}

func (l *logger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	sql, rows := fc()

	elapsed := apctx.GetClock(ctx).Since(begin)
	if err != nil {
		l.inner.Error("gorm trace failed", "sql", sql, "elapsed", elapsed, "rows", rows, "error", err)
	} else {
		l.inner.Debug("gorm trace success", "sql", sql, "elapsed", elapsed, "rows", rows, "error", err)
	}
}

func (l *logger) LogMode(glogger.LogLevel) glogger.Interface {
	// We don't allow you to change the log level
	return l
}

var _ glogger.Interface = (*logger)(nil)
