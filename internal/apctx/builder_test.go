package apctx

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
	clocktest "k8s.io/utils/clock/testing"
)

type testIdGenerator struct {
	s string
}

func (g *testIdGenerator) New(prefix apid.Prefix) apid.ID  { return apid.ID(g.s) }
func (g *testIdGenerator) NewString(prefix apid.Prefix) string { return g.s }

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
	t.Run("WithIdGenerator", func(t *testing.T) {
		gen := &testIdGenerator{s: "act_helloworld00001"}

		ctx := NewBuilderBackground().WithIdGenerator(gen).Build()

		// ensure our generator is returned
		require.Equal(t, "act_helloworld00001", GetIdGenerator(ctx).NewString(apid.PrefixActor))
	})
	t.Run("WithFixedIdGenerator", func(t *testing.T) {
		id := apid.MustParse("act_testvalue0000001")
		ctx := NewBuilderBackground().WithFixedIdGenerator(id).Build()
		require.Equal(t, id, GetIdGenerator(ctx).New(apid.PrefixActor))
	})
	t.Run("WithCorrelationID", func(t *testing.T) {
		ctx := NewBuilderBackground().WithCorrelationID("cid-123").Build()
		require.Equal(t, "cid-123", CorrelationID(ctx))
	})
	t.Run("Chaining", func(t *testing.T) {
		tm := time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC)
		fake := clocktest.NewFakeClock(tm)
		gen := &testIdGenerator{s: "act_abc12300000001"}

		ctx := NewBuilderBackground().
			With(Set("k", 42)).
			WithCorrelationID("cid-xyz").
			WithClock(fake).
			WithIdGenerator(gen).
			Build()

		require.Equal(t, 42, ctx.Value("k"))
		require.Equal(t, "cid-xyz", CorrelationID(ctx))
		require.Equal(t, tm, GetClock(ctx).Now())
		require.Equal(t, "act_abc12300000001", GetIdGenerator(ctx).NewString(apid.PrefixActor))
	})
}
