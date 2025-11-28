package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoerceBool(t *testing.T) {
	t.Parallel()
	require.True(t, CoerceBool(ToPtr(true)))
	require.False(t, CoerceBool(ToPtr(false)))
	require.False(t, CoerceBool(nil))
}

func TestCoerceString(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", CoerceString(ToPtr("")))
	require.Equal(t, "foo", CoerceString(ToPtr("foo")))
	require.Equal(t, "", CoerceString(nil))
}
