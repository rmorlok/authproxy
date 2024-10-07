package context

import (
	"context"
	"k8s.io/utils/clock"
)

const (
	clockKey = "clock"
)

// WithApplier is a function that will apply a value to a context using WithValue and return the updated
// context. This is used so that other object can inject themselves in to the .With(...) chaining of the
// context without the context taking a dependency on them.
type WithApplier interface {
	ContextWith(ctx context.Context) context.Context
}

type Context interface {
	context.Context
	With(wa WithApplier) Context
	WithClock(clock clock.Clock) Context
	Clock() clock.Clock
}

type commonContext struct {
	context.Context
}

func AsContext(ctx context.Context) Context {
	return &commonContext{
		ctx,
	}
}

func (cc *commonContext) WithClock(clock clock.Clock) Context {
	ctx := context.WithValue(cc, clockKey, clock)
	return AsContext(ctx)
}

func (cc *commonContext) Clock() clock.Clock {
	val := cc.Value(clockKey)
	if val == nil {
		return clock.RealClock{}
	}

	return val.(clock.Clock)
}

func (cc *commonContext) With(wa WithApplier) Context {
	return AsContext(wa.ContextWith(cc))
}

type valueApplier struct {
	key   string
	value interface{}
}

func (va *valueApplier) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, va.key, va.value)
}

// Set allows you to take an arbitrary key and value and use it in With(...) chaining on the context.
//
// e.g. ctx := context.Context().
//
//	With(util.Set("dog", "woof")).
//	With(util.Set("cat", "meow"))
func Set(key string, value interface{}) WithApplier {
	return &valueApplier{key, value}
}

func TODO() Context {
	return AsContext(context.TODO())
}

func Background() Context {
	return AsContext(context.Background())
}
