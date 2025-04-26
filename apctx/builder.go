package apctx

import (
	"context"
	"k8s.io/utils/clock"
)

// WithApplier is a function that will apply a value to a context using WithValue and return the updated
// context. This is used so that other object can inject themselves in to the .With(...) chaining of the
// context without the context taking a dependency on them.
type WithApplier interface {
	ContextWith(ctx context.Context) context.Context
}

type Builder interface {
	With(wa WithApplier) Builder
	WithClock(clock clock.Clock) Builder
	WithUuidGenerator(generator UuidGenerator) Builder
	Build() context.Context
}

type builder struct {
	ctx context.Context
}

func (b *builder) With(wa WithApplier) Builder {
	return &builder{wa.ContextWith(b.ctx)}
}

func (b *builder) WithClock(clock clock.Clock) Builder {
	return &builder{WithClock(b.ctx, clock)}
}

func (b *builder) WithUuidGenerator(generator UuidGenerator) Builder {
	return &builder{WithUuidGenerator(b.ctx, generator)}
}

func (b *builder) Build() context.Context {
	return b.ctx
}

func NewBuilder(ctx context.Context) Builder {
	return &builder{ctx}
}

func NewBuilderBackground() Builder {
	return NewBuilder(context.Background())
}
