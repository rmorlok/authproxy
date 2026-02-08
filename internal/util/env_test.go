package util

import (
	"os"
	"testing"
)

func TestGetEnvDefault(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		fallback string
		unset    bool
		want     string
	}{
		{
			name:     "set",
			key:      "TEST_ENV_SET",
			value:    "actual",
			fallback: "default",
			want:     "actual",
		},
		{
			name:     "unset",
			key:      "TEST_ENV_UNSET",
			fallback: "default",
			unset:    true,
			want:     "default",
		},
		{
			name:     "empty",
			key:      "TEST_ENV_EMPTY",
			value:    "",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "whitespace",
			key:      "TEST_ENV_SPACE",
			value:    "   ",
			fallback: "default",
			want:     "default",
		},
		{
			name:     "trimmed",
			key:      "TEST_ENV_TRIM",
			value:    "  trimmed  ",
			fallback: "default",
			want:     "trimmed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				os.Unsetenv(tt.key)
			} else {
				os.Setenv(tt.key, tt.value)
				defer os.Unsetenv(tt.key)
			}

			if got := GetEnvDefault(tt.key, tt.fallback); got != tt.want {
				t.Errorf("GetEnvDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}
