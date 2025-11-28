package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTabs2Spaces(t *testing.T) {
	t.Parallel()
	assert := require.New(t)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no tabs",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "single tab",
			input:    "hello\tworld",
			expected: "hello    world",
		},
		{
			name:     "multiple tabs",
			input:    "\thello\tworld\t",
			expected: "    hello    world    ",
		},
		{
			name:     "mixed whitespace",
			input:    "  \thello\t  world",
			expected: "      hello      world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TabsToSpaces(tt.input, 4)
			assert.Equal(tt.expected, result)
		})
	}
}
