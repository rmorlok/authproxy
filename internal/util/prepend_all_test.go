package util

import (
	"reflect"
	"testing"
)

func TestPrependAll(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		strs   []string
		want   []string
	}{
		{
			name:   "standard",
			prefix: "test_",
			strs:   []string{"a", "b", "c"},
			want:   []string{"test_a", "test_b", "test_c"},
		},
		{
			name:   "no values",
			prefix: "prefix_",
			strs:   []string{},
			want:   []string{},
		},
		{
			name:   "nil slice",
			prefix: "prefix_",
			strs:   nil,
			want:   []string{},
		},
		{
			name:   "empty prefix",
			prefix: "",
			strs:   []string{"a", "b", "c"},
			want:   []string{"a", "b", "c"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := PrependAll(test.prefix, test.strs)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("PrependAll(%q, %v) = %v, want %v", test.prefix, test.strs, got, test.want)
			}
		})
	}
}
