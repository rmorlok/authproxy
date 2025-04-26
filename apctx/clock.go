package apctx

import (
	"context"
	"k8s.io/utils/clock"
)

const (
	clockKey = "clock"
)

func WithClock(ctx context.Context, clock clock.Clock) context.Context {
	return context.WithValue(ctx, clockKey, clock)
}

var realClock = clock.RealClock{}

// GetClock retrieves a clock that has been set on the context. If no value has been set, it returns a real clock.
func GetClock(ctx context.Context) clock.Clock {
	val := ctx.Value(clockKey)
	if val == nil {
		return realClock
	}

	return val.(clock.Clock)
}
