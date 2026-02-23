package apctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsDebugMode_DefaultFalse(t *testing.T) {
	ctx := context.Background()
	require.False(t, IsDebugMode(ctx))
}

func TestWithDebugMode_True(t *testing.T) {
	ctx := WithDebugMode(context.Background(), true)
	require.True(t, IsDebugMode(ctx))
}

func TestWithDebugMode_False(t *testing.T) {
	ctx := WithDebugMode(context.Background(), false)
	require.False(t, IsDebugMode(ctx))
}

func TestWithDebugMode_Override(t *testing.T) {
	ctx := WithDebugMode(context.Background(), true)
	require.True(t, IsDebugMode(ctx))

	ctx = WithDebugMode(ctx, false)
	require.False(t, IsDebugMode(ctx))
}
