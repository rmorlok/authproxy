package apctx

import (
	"context"
)

const debugModeKey = "debugMode"

func WithDebugMode(ctx context.Context, debug bool) context.Context {
	return context.WithValue(ctx, debugModeKey, debug)
}

func IsDebugMode(ctx context.Context) bool {
	if debug, ok := ctx.Value(debugModeKey).(bool); ok {
		return debug
	}
	return false
}
