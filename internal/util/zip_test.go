package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZipToMap(t *testing.T) {
	tests := []struct {
		name     string
		l1       []string
		l2       []int
		expected map[string]int
	}{
		{
			name:     "zip two lists",
			l1:       []string{"a", "b", "c"},
			l2:       []int{1, 2, 3},
			expected: map[string]int{"a": 1, "b": 2, "c": 3},
		},
		{
			name:     "keys shorter",
			l1:       []string{"a", "b"},
			l2:       []int{1, 2, 3},
			expected: map[string]int{"a": 1, "b": 2},
		},
		{
			name:     "values shorter",
			l1:       []string{"a", "b", "c"},
			l2:       []int{2, 3},
			expected: map[string]int{"a": 2, "b": 3},
		},
		{
			name:     "empty",
			l1:       []string{},
			l2:       []int{},
			expected: map[string]int{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, ZipToMap(test.l1, test.l2))
		})
	}
}
