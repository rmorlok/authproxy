package context

import (
	"context"
	"github.com/google/uuid"
	"k8s.io/utils/clock"
	"time"
)

const (
	clockKey         = "clock"
	uuidGeneratorKey = "uuidGenerator"
)

// WithApplier is a function that will apply a value to a context using WithValue and return the updated
// context. This is used so that other object can inject themselves in to the .With(...) chaining of the
// context without the context taking a dependency on them.
type WithApplier interface {
	ContextWith(ctx context.Context) context.Context
}

// UuidGenerator is an interface to an object that will provide random UUIDs (UUID1, UUID4).
// The default implementation will delegate to the google uuid package. This allows deterministic
// random UUID generation in tests by mocking this interface. For deterministic UUIDs (UUID3, UUID5)
// code should use the UUID package directly.
type UuidGenerator interface {
	// NewUUID returns a Version 1 UUID based on the current NodeID and clock
	// sequence, and the current time.  If the NodeID has not been set by SetNodeID
	// or SetNodeInterface then it will be set automatically.  If the NodeID cannot
	// be set NewUUID returns nil.  If clock sequence has not been set by
	// SetClockSequence then it will be set automatically.  If GetTime fails to
	// return the current NewUUID returns nil and an error.
	//
	// In most cases, New should be used.
	NewUUID() (uuid.UUID, error)

	// New creates a new random UUID or panics.  New is equivalent to
	// the expression
	//
	//    uuid.Must(uuid.NewRandom())
	New() uuid.UUID

	// NewString creates a new random UUID and returns it as a string or panics.
	// NewString is equivalent to the expression
	//
	// New().String()
	NewString() string
}

type Context interface {
	context.Context
	With(wa WithApplier) Context
	WithClock(clock clock.Clock) Context
	Clock() clock.Clock
	WithUuidGenerator(generator UuidGenerator) Context
	UuidGenerator() UuidGenerator
}

type CancelFunc = context.CancelFunc

type commonContext struct {
	context.Context
}

func AsContext(ctx context.Context) Context {
	return &commonContext{
		ctx,
	}
}

func WithDeadline(ctx context.Context, deadline time.Time) (Context, context.CancelFunc) {
	out, cancel := context.WithDeadline(ctx, deadline)
	return AsContext(out), cancel
}

func WithTimeout(ctx context.Context, timeout time.Duration) (Context, context.CancelFunc) {
	deadline := time.Now().Add(timeout)
	if innerCtx, ok := ctx.(Context); ok {
		deadline = innerCtx.Clock().Now().Add(timeout)
	}

	out, cancel := WithDeadline(ctx, deadline)
	return AsContext(out), cancel
}

func WithCancel(ctx context.Context) (Context, CancelFunc) {
	out, cancel := context.WithCancel(ctx)
	return AsContext(out), cancel
}

func (cc *commonContext) WithClock(clock clock.Clock) Context {
	ctx := context.WithValue(cc, clockKey, clock)
	return AsContext(ctx)
}

var realClock = clock.RealClock{}

func (cc *commonContext) Clock() clock.Clock {
	val := cc.Value(clockKey)
	if val == nil {
		return realClock
	}

	return val.(clock.Clock)
}

func (cc *commonContext) WithUuidGenerator(generator UuidGenerator) Context {
	ctx := context.WithValue(cc, uuidGeneratorKey, generator)
	return AsContext(ctx)
}

type realUuidGenerator struct{}

func (g *realUuidGenerator) NewUUID() (uuid.UUID, error) {
	return uuid.NewUUID()
}

func (g *realUuidGenerator) New() uuid.UUID {
	return uuid.New()
}

func (g *realUuidGenerator) NewString() string {
	return uuid.NewString()
}

var realUuidGeneratorVal UuidGenerator = &realUuidGenerator{}

func (cc *commonContext) UuidGenerator() UuidGenerator {
	val := cc.Value(uuidGeneratorKey)
	if val == nil {
		return realUuidGeneratorVal
	}

	return val.(UuidGenerator)
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
