package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name      string
		l1        []string
		predicate func(string) bool
		expected  []string
	}{
		{
			name:      "empty, all filtered",
			l1:        []string{},
			predicate: func(s string) bool { return false },
			expected:  nil,
		},
		{
			name:      "empty, none filtered",
			l1:        []string{},
			predicate: func(s string) bool { return true },
			expected:  nil,
		},
		{
			name:      "none filtered",
			l1:        []string{"a", "b", "c"},
			predicate: func(s string) bool { return true },
			expected:  []string{"a", "b", "c"},
		},
		{
			name:      "all filtered",
			l1:        []string{"a", "b", "c"},
			predicate: func(s string) bool { return false },
			expected:  nil,
		},
		{
			name:      "some filtered",
			l1:        []string{"a", "b", "c"},
			predicate: func(s string) bool { return s != "b" },
			expected:  []string{"a", "c"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, Filter(test.l1, test.predicate))
		})
	}
}
