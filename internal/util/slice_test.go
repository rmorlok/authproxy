package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountBy(t *testing.T) {
	cases := []struct {
		items []string
		equal string
		exp   int
	}{
		{nil, "a", 0},
		{[]string{}, "a", 0},
		{[]string{"a"}, "a", 1},
		{[]string{"a", "b"}, "a", 1},
		{[]string{"a", "a"}, "a", 2},
		{[]string{"b", "b"}, "a", 0},
	}

	for _, tc := range cases {
		require.Equal(t, tc.exp, CountBy(tc.items, func(b string) bool { return b == tc.equal }))
	}
}

func TestCountTrue(t *testing.T) {
	cases := []struct {
		items []bool
		exp   int
	}{
		{nil, 0},
		{[]bool{}, 0},
		{[]bool{true}, 1},
		{[]bool{false}, 0},
		{[]bool{true, false}, 1},
		{[]bool{true, true}, 2},
		{[]bool{false, false}, 0},
	}

	for _, tc := range cases {
		require.Equal(t, tc.exp, CountTrue(tc.items))
	}
}

func TestPrependStringToAll(t *testing.T) {
	cases := []struct {
		items []string
		exp   []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"x"}, []string{"ax"}},
		{[]string{"x", "y"}, []string{"ax", "ay"}},
	}
	for _, tc := range cases {
		require.ElementsMatch(t, tc.exp, PrependStringToAll("a", tc.items))
	}
}

func TestAppendStringToAll(t *testing.T) {
	cases := []struct {
		items []string
		exp   []string
	}{
		{nil, nil},
		{[]string{}, []string{}},
		{[]string{"x"}, []string{"xa"}},
		{[]string{"x", "y"}, []string{"xa", "ya"}},
	}
	for _, tc := range cases {
		require.ElementsMatch(t, tc.exp, AppendStringToAll("a", tc.items))
	}
}
