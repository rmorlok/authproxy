package util

import (
	"bytes"
	"reflect"
	"testing"
)

func TestJsonToReader(t *testing.T) {
	tests := []struct {
		name      string
		input     interface{}
		expected  string
		wantPanic bool
	}{
		{
			name:      "simple struct",
			input:     struct{ Name string }{Name: "Test"},
			expected:  `{"Name":"Test"}`,
			wantPanic: false,
		},
		{
			name:      "empty struct",
			input:     struct{}{},
			expected:  `{}`,
			wantPanic: false,
		},
		{
			name:      "nil value",
			input:     nil,
			expected:  `null`,
			wantPanic: false,
		},
		{
			name:      "slice of integers",
			input:     []int{1, 2, 3},
			expected:  `[1,2,3]`,
			wantPanic: false,
		},
		{
			name:      "map of strings to integers",
			input:     map[string]int{"one": 1, "two": 2},
			expected:  `{"one":1,"two":2}`,
			wantPanic: false,
		},
		{
			name:      "unsupported type (channel)",
			input:     make(chan int),
			expected:  "",
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.wantPanic {
						t.Errorf("unexpected panic: %v", r)
					}
				} else if tt.wantPanic {
					t.Errorf("expected panic, but did not panic")
				}
			}()

			result := JsonToReader(tt.input)
			if !tt.wantPanic {
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(result); err != nil {
					t.Fatalf("failed to read result: %v", err)
				}

				got := buf.String()
				if !reflect.DeepEqual(got, tt.expected) {
					t.Errorf("JsonToReader() = %v, want %v", got, tt.expected)
				}
			}
		})
	}
}
