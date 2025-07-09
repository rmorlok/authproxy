package mock

import (
	"context"
	"log/slog"
	"testing"
)

// TestingHandler captures logs for testing
type TestingHandler struct {
	Logs []LogEntry
	TB   testing.TB
}

type LogEntry struct {
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

func (h *TestingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return true
}

func (h *TestingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0)
	r.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})

	h.Logs = append(h.Logs, LogEntry{
		Level:   r.Level,
		Message: r.Message,
		Attrs:   attrs,
	})
	return nil
}

func (h *TestingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *TestingHandler) WithGroup(name string) slog.Handler {
	return h
}

// Creates a new test logger
func NewTestLogger(tb testing.TB) (*slog.Logger, *TestingHandler) {
	handler := &TestingHandler{TB: tb}
	return slog.New(handler), handler
}
