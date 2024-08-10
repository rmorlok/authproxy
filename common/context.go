package common

import (
	"context"
	"k8s.io/utils/clock"
)

const (
	clockKey = "clock"
)

type Context interface {
	context.Context
}

type commonContext struct {
	context.Context
}

func AsContext(ctx context.Context) context.Context {
	return &commonContext{
		ctx,
	}
}

func (cc *commonContext) WithClock(clock clock.Clock) Context {
	ctx := context.WithValue(cc, clockKey, clock)
	return AsContext(ctx)
}

func (cc *commonContext) GetClock() clock.Clock {
	val := cc.Value(clockKey)
	if val == nil {
		return clock.RealClock{}
	}

	return val.(clock.Clock)
}
