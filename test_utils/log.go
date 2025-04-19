package test_utils

import (
	"log/slog"
	"os"
)

func NewTestLogger() *slog.Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     slog.LevelError,
		AddSource: true,
	})

	return slog.New(handler)
}
