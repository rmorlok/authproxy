package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirst2(t *testing.T) {
	f := func() (string, int) {
		return "foo", 1
	}

	require.Equal(t, "foo", First2(f()))
}

func TestFirst3(t *testing.T) {
	f := func() (string, int, bool) {
		return "foo", 1, true
	}

	require.Equal(t, "foo", First3(f()))
}

func TestSecond2(t *testing.T) {
	f := func() (string, int) {
		return "foo", 1
	}

	require.Equal(t, 1, Second2(f()))
}

func TestSecond3(t *testing.T) {
	f := func() (string, int, bool) {
		return "foo", 1, true
	}

	require.Equal(t, 1, Second3(f()))
}

func TestThird3(t *testing.T) {
	f := func() (string, int, bool) {
		return "foo", 1, true
	}

	require.Equal(t, true, Third3(f()))
}
