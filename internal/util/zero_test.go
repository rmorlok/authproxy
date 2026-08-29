package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsZeroValue(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		isZero bool
	}{
		{
			name:   "nil value",
			value:  nil,
			isZero: true,
		},
		{
			name:   "empty string",
			value:  "",
			isZero: true,
		},
		{
			name:   "false",
			value:  false,
			isZero: true,
		},
		{
			name:   "zero",
			value:  int64(0),
			isZero: true,
		},
		{
			name:   "empty struct",
			value:  struct{}{},
			isZero: true,
		},
		{
			name:   "ptr value",
			value:  ToPtr("foo"),
			isZero: false,
		},
		{
			name:   "populated string",
			value:  "foo",
			isZero: false,
		},
		{
			name:   "true",
			value:  true,
			isZero: false,
		},
		{
			name:   "integer",
			value:  int64(1),
			isZero: false,
		},
		{
			name:   "populated struct",
			value:  struct{ foo string }{"foo"},
			isZero: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.isZero, IsZeroValue(tc.value))
		})
	}
}
