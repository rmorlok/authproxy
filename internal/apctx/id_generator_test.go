package apctx

import (
	"context"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/require"
)

func TestGetIdGenerator(t *testing.T) {
	t.Run("default id generator", func(t *testing.T) {
		ctx := context.Background()

		require.NotNil(t, GetIdGenerator(ctx))
		require.False(t, GetIdGenerator(ctx).New(apid.PrefixActor).IsNil())
		require.NotEmpty(t, GetIdGenerator(ctx).NewString(apid.PrefixActor))

		require.NotEqual(t, GetIdGenerator(ctx).New(apid.PrefixActor), GetIdGenerator(ctx).New(apid.PrefixActor))
		require.NotEqual(t, GetIdGenerator(ctx).NewString(apid.PrefixActor), GetIdGenerator(ctx).NewString(apid.PrefixActor))
	})

	t.Run("fixed id generator", func(t *testing.T) {
		ctx := context.Background()
		fixed := apid.MustParse("act_testfixed000001")
		ctx = WithFixedIdGenerator(ctx, fixed)

		require.NotNil(t, GetIdGenerator(ctx))
		require.Equal(t, fixed, GetIdGenerator(ctx).New(apid.PrefixActor))
		require.Equal(t, fixed.String(), GetIdGenerator(ctx).NewString(apid.PrefixActor))
	})
}
