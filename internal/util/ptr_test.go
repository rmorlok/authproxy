package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToPtr(t *testing.T) {
	t.Parallel()
	x := "foo"
	require.Equal(t, &x, ToPtr(x))
}
