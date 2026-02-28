package util

import (
	"testing"
)

func TestSameInstance(t *testing.T) {
	type testStruct struct {
		value int
	}

	// Table-driven tests
	tests := []struct {
		name     string
		a, b     any
		expected bool
	}{
		{
			name:     "nil values",
			a:        nil,
			b:        nil,
			expected: false,
		},
		{
			name:     "first nil, second non-nil",
			a:        nil,
			b:        &testStruct{value: 1},
			expected: false,
		},
		{
			name:     "first non-nil, second nil",
			a:        &testStruct{value: 1},
			b:        nil,
			expected: false,
		},
		{
			name:     "different pointer instances, same value",
			a:        &testStruct{value: 42},
			b:        &testStruct{value: 42},
			expected: false,
		},
		{
			name:     "same pointer instance",
			a:        func() any { s := &testStruct{value: 42}; return s }(),
			b:        func() any { s := &testStruct{value: 42}; return s }(),
			expected: true,
		},
		{
			name:     "non-pointer values",
			a:        testStruct{value: 42},
			b:        testStruct{value: 42},
			expected: false,
		},
		{
			name:     "pointer and non-pointer mismatch",
			a:        &testStruct{value: 5},
			b:        testStruct{value: 5},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SameInstance(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("SameInstance(%v, %v) = %v; want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
