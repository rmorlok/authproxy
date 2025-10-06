package apctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueApplier(t *testing.T) {
	ctx := context.Background()

	applier := Set("foo", "bar")
	require.NotNil(t, applier)

	ctx = NewBuilder(ctx).With(applier).Build()
	require.Equal(t, "bar", ctx.Value("foo"))
}
