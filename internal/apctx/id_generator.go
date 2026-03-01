package apctx

import (
	"context"

	"github.com/rmorlok/authproxy/internal/apid"
)

const (
	idGeneratorKey = "idGenerator"
)

// IdGenerator is an interface to an object that will provide random prefixed IDs.
// The default implementation delegates to apid.New(). This allows deterministic
// ID generation in tests by using WithFixedIdGenerator.
type IdGenerator interface {
	// New creates a new random ID with the given prefix.
	New(prefix apid.Prefix) apid.ID

	// NewString creates a new random ID with the given prefix and returns it as a string.
	NewString(prefix apid.Prefix) string
}

type realIdGenerator struct{}

func (g *realIdGenerator) New(prefix apid.Prefix) apid.ID {
	return apid.New(prefix)
}

func (g *realIdGenerator) NewString(prefix apid.Prefix) string {
	return apid.New(prefix).String()
}

var realIdGeneratorVal IdGenerator = &realIdGenerator{}

// GetIdGenerator retrieves an ID generator from the context if one has been set. If not, it returns the real ID
// generator.
func GetIdGenerator(ctx context.Context) IdGenerator {
	val := ctx.Value(idGeneratorKey)
	if val == nil {
		return realIdGeneratorVal
	}

	return val.(IdGenerator)
}

// WithIdGenerator sets an ID generator on the context.
func WithIdGenerator(ctx context.Context, generator IdGenerator) context.Context {
	return context.WithValue(ctx, idGeneratorKey, generator)
}

type fixedIdGenerator struct {
	id apid.ID
}

func (g *fixedIdGenerator) New(prefix apid.Prefix) apid.ID {
	return g.id
}

func (g *fixedIdGenerator) NewString(prefix apid.Prefix) string {
	return g.id.String()
}

// WithFixedIdGenerator sets a fixed ID generator on the context that will always return the same ID.
func WithFixedIdGenerator(ctx context.Context, id apid.ID) context.Context {
	return WithIdGenerator(ctx, &fixedIdGenerator{id: id})
}
