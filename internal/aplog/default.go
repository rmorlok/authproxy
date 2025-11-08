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
