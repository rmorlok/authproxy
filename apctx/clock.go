package apctx

import (
	"context"
	"time"

	"k8s.io/utils/clock"
	tclock "k8s.io/utils/clock/testing"
)

const (
	clockKey = "clock"
)

// WithClock sets a clock on the context.
func WithClock(ctx context.Context, clock clock.Clock) context.Context {
	return context.WithValue(ctx, clockKey, clock)
}

// WithFixedClock sets a fixed clock on the context that will always return the same time.
func WithFixedClock(ctx context.Context, t time.Time) context.Context {
	return WithClock(ctx, tclock.NewFakeClock(t))
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
