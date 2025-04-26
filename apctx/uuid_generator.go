package apctx

import (
	"context"
	"github.com/google/uuid"
)

const (
	uuidGeneratorKey = "uuidGenerator"
)

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

// GetUuidGenerator retrieves a UUID generator from the context if one has been set. If not, it returns the real UUID
// generator.
func GetUuidGenerator(ctx context.Context) UuidGenerator {
	val := ctx.Value(uuidGeneratorKey)
	if val == nil {
		return realUuidGeneratorVal
	}

	return val.(UuidGenerator)
}

func WithUuidGenerator(ctx context.Context, generator UuidGenerator) context.Context {
	return context.WithValue(ctx, uuidGeneratorKey, generator)
}
