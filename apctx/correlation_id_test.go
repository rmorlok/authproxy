package apctx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCorrelationID(t *testing.T) {
	ctx := context.Background()
	require.Empty(t, CorrelationID(ctx))

	ctx = WithCorrelationID(ctx, "some-value")
	require.Equal(t, "some-value", CorrelationID(ctx))
}
