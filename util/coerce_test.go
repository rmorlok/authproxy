package util

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCoerceBool(t *testing.T) {
	require.True(t, CoerceBool(ToPtr(true)))
	require.False(t, CoerceBool(ToPtr(false)))
	require.False(t, CoerceBool(nil))
}

func TestCoerceString(t *testing.T) {
	require.Equal(t, "", CoerceString(ToPtr("")))
	require.Equal(t, "foo", CoerceString(ToPtr("foo")))
	require.Equal(t, "", CoerceString(nil))
}
