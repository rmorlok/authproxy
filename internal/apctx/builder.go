package apctx

import (
	"context"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"k8s.io/utils/clock"
)

// WithApplier is a function that will apply a value to a context using WithValue and return the updated
// context. This is used so that other objects can inject themselves in to the .With(...) chaining of the
// context without the context taking a dependency on them.
type WithApplier interface {
	ContextWith(ctx context.Context) context.Context
}

// Builder is an object that can apply multiple transformations and emit a context with those settings.
type Builder interface {
	With(wa WithApplier) Builder
	WithClock(clock clock.Clock) Builder
	WithFixedClock(t time.Time) Builder
	WithIdGenerator(generator IdGenerator) Builder
	WithFixedIdGenerator(id apid.ID) Builder
	WithCorrelationID(correlationID string) Builder
	WithDebugMode(debug bool) Builder
	Build() context.Context
}

type builder struct {
	ctx context.Context
}

// With applies a WithApplier to the context.
func (b *builder) With(wa WithApplier) Builder {
	return &builder{wa.ContextWith(b.ctx)}
}

// WithClock sets a clock on the context.
func (b *builder) WithClock(clock clock.Clock) Builder {
	return &builder{WithClock(b.ctx, clock)}
}

// WithFixedClock sets a fixed clock on the context that will always return the same time.
func (b *builder) WithFixedClock(t time.Time) Builder {
	return &builder{WithFixedClock(b.ctx, t)}
}

// WithIdGenerator sets an ID generator on the context.
func (b *builder) WithIdGenerator(generator IdGenerator) Builder {
	return &builder{WithIdGenerator(b.ctx, generator)}
}

// WithFixedIdGenerator sets a fixed ID generator on the context that will always return the same ID.
func (b *builder) WithFixedIdGenerator(id apid.ID) Builder {
	return &builder{WithFixedIdGenerator(b.ctx, id)}
}

// WithCorrelationID sets a correlation ID on the context.
func (b *builder) WithCorrelationID(correlationId string) Builder {
	return &builder{WithCorrelationID(b.ctx, correlationId)}
}

// WithDebugMode sets the debug mode flag on the context.
func (b *builder) WithDebugMode(debug bool) Builder {
	return &builder{WithDebugMode(b.ctx, debug)}
}

// Build returns the context.
func (b *builder) Build() context.Context {
	return b.ctx
}

// NewBuilder creates a new builder with the given context.
func NewBuilder(ctx context.Context) Builder {
	return &builder{ctx}
}

// NewBuilderBackground creates a new builder with the background context.
func NewBuilderBackground() Builder {
	return NewBuilder(context.Background())
}
