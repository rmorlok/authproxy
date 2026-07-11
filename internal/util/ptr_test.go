package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToPtr(t *testing.T) {
	t.Parallel()
	x := "foo"
	require.Equal(t, &x, ToPtr(x))

	y := ""
	require.Equal(t, &y, ToPtr(y))
}

func TestToPtrNonZero(t *testing.T) {
	t.Parallel()
	x := "foo"
	require.Equal(t, &x, ToPtrNonZero(x))

	y := ""
	require.Nil(t, ToPtrNonZero(y))

	z := 0
	require.Nil(t, ToPtrNonZero(z))
}
