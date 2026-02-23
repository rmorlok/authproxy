package apctx

import (
	"context"
	"time"

	"github.com/google/uuid"
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
	WithUuidGenerator(generator UuidGenerator) Builder
	WithFixedUuidGenerator(u uuid.UUID) Builder
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

// WithUuidGenerator sets a UUID generator on the context.
func (b *builder) WithUuidGenerator(generator UuidGenerator) Builder {
	return &builder{WithUuidGenerator(b.ctx, generator)}
}

// WithFixedUuidGenerator sets a fixed UUID generator on the context that will always return the same UUID.
func (b *builder) WithFixedUuidGenerator(u uuid.UUID) Builder {
	return &builder{WithFixedUuidGenerator(b.ctx, u)}
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
