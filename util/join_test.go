package util

import (
	"testing"
)

func TestStringsJoin(t *testing.T) {
	tests := []struct {
		name     string
		strs     []string
		sep      string
		expected string
	}{
		{
			name:     "empty slice",
			strs:     []string{},
			sep:      ",",
			expected: "",
		},
		{
			name:     "single element",
			strs:     []string{"one"},
			sep:      ",",
			expected: "one",
		},
		{
			name:     "multiple elements with separator",
			strs:     []string{"one", "two", "three"},
			sep:      ",",
			expected: "one,two,three",
		},
		{
			name:     "multiple elements with empty separator",
			strs:     []string{"one", "two", "three"},
			sep:      "",
			expected: "onetwothree",
		},
		{
			name:     "empty strings in slice",
			strs:     []string{"", "", ""},
			sep:      ",",
			expected: ",,",
		},
		{
			name:     "empty separator with empty strings",
			strs:     []string{"", "", ""},
			sep:      "",
			expected: "",
		},
		{
			name:     "unicode characters",
			strs:     []string{"你好", "世界"},
			sep:      " ",
			expected: "你好 世界",
		},
		{
			name:     "special characters",
			strs:     []string{"!@#", "$%^", "&*()"},
			sep:      "-",
			expected: "!@#-$%^-&*()",
		},
		{
			name:     "separator only",
			strs:     []string{"one", "two", "three"},
			sep:      "-sep-",
			expected: "one-sep-two-sep-three",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringsJoin(tt.strs, tt.sep)
			if result != tt.expected {
				t.Errorf("StringsJoin(%v, %q) = %q; want %q", tt.strs, tt.sep, result, tt.expected)
			}
		})
	}
}
