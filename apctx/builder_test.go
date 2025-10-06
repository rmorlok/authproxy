package apctx

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	clocktest "k8s.io/utils/clock/testing"
)

type testUuidGenerator struct {
	s string
}

func (g *testUuidGenerator) NewUUID() (uuid.UUID, error) { return uuid.UUID{}, nil }
func (g *testUuidGenerator) New() uuid.UUID              { return uuid.UUID{} }
func (g *testUuidGenerator) NewString() string           { return g.s }

func TestBuilder(t *testing.T) {
	t.Run("NewBuilderAndBackground", func(t *testing.T) {
		base := context.WithValue(context.Background(), "init", "ok")

		// NewBuilder keeps base context
		ctx := NewBuilder(base).Build()
		require.Equal(t, "ok", ctx.Value("init"))

		// Background creates from context.Background()
		bg := NewBuilderBackground().Build()
		require.NotNil(t, bg)
		require.Nil(t, bg.Value("init"))
	})
	t.Run("With_Applier", func(t *testing.T) {
		ctx := context.Background()

		// apply using Set WithApplier
		ctx = NewBuilder(ctx).With(Set("foo", "bar")).Build()
		require.Equal(t, "bar", ctx.Value("foo"))
	})
	t.Run("WithClock", func(t *testing.T) {
		tm := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		fake := clocktest.NewFakeClock(tm)

		ctx := NewBuilderBackground().WithClock(fake).Build()

		// GetClock should return our fake clock
		require.Equal(t, tm, GetClock(ctx).Now())
	})
	t.Run("WithFixedClock", func(t *testing.T) {
		tm := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		ctx := NewBuilderBackground().WithFixedClock(tm).Build()
		require.Equal(t, tm, GetClock(ctx).Now())
	})
	t.Run("WithUuidGenerator", func(t *testing.T) {
		gen := &testUuidGenerator{s: "hello-world"}

		ctx := NewBuilderBackground().WithUuidGenerator(gen).Build()

		// ensure our generator is returned
		require.Equal(t, "hello-world", GetUuidGenerator(ctx).NewString())
	})
	t.Run("WithFixedUuidGenerator", func(t *testing.T) {
		u := uuid.New()
		ctx := NewBuilderBackground().WithFixedUuidGenerator(u).Build()
		require.Equal(t, u, GetUuidGenerator(ctx).New())
	})
	t.Run("WithCorrelationID", func(t *testing.T) {
		ctx := NewBuilderBackground().WithCorrelationID("cid-123").Build()
		require.Equal(t, "cid-123", CorrelationID(ctx))
	})
	t.Run("Chaining", func(t *testing.T) {
		tm := time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC)
		fake := clocktest.NewFakeClock(tm)
		gen := &testUuidGenerator{s: "abc-123"}

		ctx := NewBuilderBackground().
			With(Set("k", 42)).
			WithCorrelationID("cid-xyz").
			WithClock(fake).
			WithUuidGenerator(gen).
			Build()

		require.Equal(t, 42, ctx.Value("k"))
		require.Equal(t, "cid-xyz", CorrelationID(ctx))
		require.Equal(t, tm, GetClock(ctx).Now())
		require.Equal(t, "abc-123", GetUuidGenerator(ctx).NewString())
	})
}
