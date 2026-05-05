package helpers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// LogCapture is a thread-safe sink for slog records emitted during a test.
// It plugs into the integration test environment via SetupOptions.LogCapture,
// replacing the configured logging block with a JSON handler that writes
// into an in-memory buffer the test can inspect after the fact.
//
// Tests use this to assert on observability events the production code
// emits — e.g., the "oauth callback rejected" events from PR 1 (#214).
// The capture sits at the root logger level, so any descendant logger
// (per-service, per-component, per-connection) routes through it.
type LogCapture struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	level  slog.Level
	logger *slog.Logger
}

// NewLogCapture builds a capture that records every record at Debug or
// above. Tests filter by level/category/etc. when reading.
func NewLogCapture() *LogCapture {
	c := &LogCapture{level: slog.LevelDebug}
	return c
}

// Records returns every captured record as a parsed map. Each call rescans
// the entire buffer — cheap relative to the rest of an integration test
// and preserves the simplest possible API.
func (c *LogCapture) Records(t *testing.T) []map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()

	records := []map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(c.buf.Bytes()))
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("LogCapture: failed to decode record: %v", err)
		}
		records = append(records, rec)
	}
	return records
}

// RejectionEvents returns just the "oauth callback rejected" events. The
// message string is the public contract from PR 1 — alerts and tests both
// key off it.
func (c *LogCapture) RejectionEvents(t *testing.T) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, r := range c.Records(t) {
		if r["msg"] == "oauth callback rejected" {
			out = append(out, r)
		}
	}
	return out
}

// asLoggingImpl returns a sconfig.LoggingImpl that produces slog.Loggers
// writing JSON records into the capture's buffer. Setup swaps this into
// cfg.GetRoot().Logging.InnerVal before constructing the dependency
// manager, so every logger derived from the root flows here.
func (c *LogCapture) asLoggingImpl() sconfig.LoggingImpl {
	return &captureLoggingImpl{capture: c}
}

type captureLoggingImpl struct {
	capture *LogCapture
}

func (l *captureLoggingImpl) GetType() sconfig.LoggingConfigType {
	return sconfig.LoggingConfigTypeJson
}

func (l *captureLoggingImpl) GetRootLogger() *slog.Logger {
	if l.capture.logger != nil {
		return l.capture.logger
	}
	handler := slog.NewJSONHandler(
		&lockedWriter{cap: l.capture},
		&slog.HandlerOptions{Level: l.capture.level},
	)
	l.capture.logger = slog.New(handler)
	return l.capture.logger
}

// lockedWriter is the sink behind the JSON handler — the slog handler is
// safe under concurrent use only when its writer is, so we lock around
// every Write to keep records non-interleaved.
type lockedWriter struct {
	cap *LogCapture
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.cap.mu.Lock()
	defer w.cap.mu.Unlock()
	return w.cap.buf.Write(p)
}
