package util

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestToPtr(t *testing.T) {
	x := "foo"
	require.Equal(t, &x, ToPtr(x))
}
