package aplog

import (
	"context"
	"log/slog"
)

type NoopHandler struct{}

func (h *NoopHandler) Enabled(_ context.Context, _ slog.Level) bool  { return false }
func (h *NoopHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h *NoopHandler) WithAttrs(_ []slog.Attr) slog.Handler          { return h }
func (h *NoopHandler) WithGroup(_ string) slog.Handler               { return h }

func NewNoopLogger() *slog.Logger {
	return slog.New(&NoopHandler{})
}
