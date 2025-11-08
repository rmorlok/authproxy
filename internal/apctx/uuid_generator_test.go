package apctx

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetUuidGenerator(t *testing.T) {
	t.Run("default uuid generator", func(t *testing.T) {
		ctx := context.Background()

		require.NotNil(t, GetUuidGenerator(ctx))
		require.NotEmpty(t, GetUuidGenerator(ctx).New())
		require.NotEmpty(t, GetUuidGenerator(ctx).NewString())

		u, err := GetUuidGenerator(ctx).NewUUID()
		require.NoError(t, err)
		require.NotEmpty(t, u)

		require.NotEqual(t, GetUuidGenerator(ctx).New(), GetUuidGenerator(ctx).New())
		require.NotEqual(t, GetUuidGenerator(ctx).NewString(), GetUuidGenerator(ctx).NewString())

		u2, err := GetUuidGenerator(ctx).NewUUID()
		require.NoError(t, err)
		require.NotEqual(t, u, u2)
	})

	t.Run("fixed uuid generator", func(t *testing.T) {
		ctx := context.Background()
		fixed := uuid.MustParse("00000000-0000-0000-0000-000000000001")
		ctx = WithFixedUuidGenerator(ctx, fixed)

		require.NotNil(t, GetUuidGenerator(ctx))
		require.Equal(t, fixed, GetUuidGenerator(ctx).New())
		require.Equal(t, fixed.String(), GetUuidGenerator(ctx).NewString())

		u, err := GetUuidGenerator(ctx).NewUUID()
		require.NoError(t, err)
		require.Equal(t, fixed, u)
	})
}
