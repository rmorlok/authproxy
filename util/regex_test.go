package util

import (
	"testing"
)

func TestEscapeRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no_special_characters",
			input:    "normalstring",
			expected: "normalstring",
		},
		{
			name:     "escape_backslash",
			input:    `a\b\c`,
			expected: `a\\b\\c`,
		},
		{
			name:     "escape_dot",
			input:    `filename.ext`,
			expected: `filename\.ext`,
		},
		{
			name:     "escape_plus_symbol",
			input:    `a+b`,
			expected: `a\+b`,
		},
		{
			name:     "escape_asterisk",
			input:    `a*b`,
			expected: `a\*b`,
		},
		{
			name:     "escape_question_mark",
			input:    `what?`,
			expected: `what\?`,
		},
		{
			name:     "escape_square_brackets",
			input:    `[abc]`,
			expected: `\[abc\]`,
		},
		{
			name:     "escape_curly_braces",
			input:    `{key:value}`,
			expected: `\{key:value\}`,
		},
		{
			name:     "escape_parentheses",
			input:    `(a|b)`,
			expected: `\(a|b\)`,
		},
		{
			name:     "escape_dollar_sign",
			input:    `price$`,
			expected: `price\$`,
		},
		{
			name:     "mixed_special_characters",
			input:    `a+b*c?d[e]f{g}(h)$i`,
			expected: `a\+b\*c\?d\[e\]f\{g\}\(h\)\$i`,
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "string_with_only_special_characters",
			input:    `.+*?[]{}()$`,
			expected: `\.\+\*\?\[\]\{\}\(\)\$`,
		},
		{
			name:     "complex_string_with_repeated_special_characters",
			input:    `\.\+\*\?\[\]\{\}\(\)\$\.\+\*\?\[\]\{\}\(\)\$`,
			expected: `\\\.\+\*\?\[\]\{\}\(\)\$\\\.\+\*\?\[\]\{\}\(\)\$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeRegex(tt.input)
			if result != tt.expected {
				t.Errorf("EscapeRegex(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
